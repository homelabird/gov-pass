//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"fk-gov/internal/driver"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	tuiExeName       = "gov-pass-tui.exe"
	winDivertSvcName = "WinDivert"
)

var (
	msiHelperKnownFolderPath = windows.KnownFolderPath
	msiHelperSystemDirectory = windows.GetSystemDirectory
	msiHelperStat            = os.Stat
)

func main() {
	action := flag.String("action", "", "action: kill-tui|purge-programdata|stop-windivert|delete-windivert|verify-windivert")
	driverDir := flag.String("windivert-dir", "", "directory containing WinDivert.dll/.sys (default: helper exe dir)")
	driverSys := flag.String("windivert-sys", "", "driver sys filename (default: WinDivert64.sys or WinDivert.sys)")
	serviceName := flag.String("service-name", winDivertSvcName, "WinDivert service name")
	flag.Parse()

	act := strings.ToLower(strings.TrimSpace(*action))
	svcName := strings.TrimSpace(*serviceName)
	if svcName == "" {
		svcName = winDivertSvcName
	}
	sysName := strings.TrimSpace(*driverSys)

	if err := driver.ValidateServiceName(svcName); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "invalid service name: %v\n", err)
		os.Exit(2)
	}
	if err := driver.ValidateDriverFileName(sysName); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "invalid driver sys name: %v\n", err)
		os.Exit(2)
	}

	switch act {
	case "kill-tui":
		_ = killTuiBestEffort()
	case "purge-programdata":
		_ = purgeProgramDataBestEffort()
	case "stop-windivert":
		_ = stopServiceBestEffort(svcName, 10*time.Second)
	case "delete-windivert":
		_ = deleteServiceBestEffort(svcName)
	case "verify-windivert":
		if err := verifyWinDivert(strings.TrimSpace(*driverDir), sysName, svcName); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	default:
		// MSI custom actions use Return="ignore", but keep a non-zero exit code
		// for manual invocation/debugging.
		_, _ = fmt.Fprintf(os.Stderr, "unknown --action: %q\n", act)
		os.Exit(2)
	}
}

func killTuiBestEffort() error {
	taskkill, err := resolveMSIHelperSystem32Command("taskkill.exe")
	if err != nil {
		return err
	}
	cmd := exec.Command(taskkill, "/IM", tuiExeName, "/F")
	// Suppress output: MSI logs would capture this, but we keep it quiet.
	cmd.Stdout = nil
	cmd.Stderr = nil
	_ = cmd.Run()
	return nil
}

func resolveMSIHelperSystem32Command(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("System32 command name is empty")
	}
	if filepath.Base(name) != name || filepath.VolumeName(name) != "" {
		return "", fmt.Errorf("System32 command name must be a bare filename: %s", name)
	}
	sysDir, err := msiHelperSystemDirectory()
	if err != nil {
		return "", fmt.Errorf("resolve System32 failed: %w", err)
	}
	sysDir = strings.TrimSpace(sysDir)
	if sysDir == "" {
		return "", errors.New("resolve System32 failed: empty path")
	}
	path := filepath.Join(sysDir, name)
	if unsafe, err := windowsPathHasReparsePoint(path); err != nil {
		return "", err
	} else if unsafe {
		return "", fmt.Errorf("System32 command must not be a reparse point: %s", path)
	}
	info, err := msiHelperStat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("System32 command is a directory: %s", path)
	}
	return path, nil
}

func purgeProgramDataBestEffort() error {
	base := defaultMSIHelperProgramDataDir()
	dir := filepath.Join(base, "gov-pass")

	// Safety guard: never remove the entire ProgramData root.
	if err := validateMSIHelperPurgeTarget(base, dir); err != nil {
		return err
	}

	if unsafe, err := windowsPathHasReparsePoint(base); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	} else if unsafe {
		return errors.New("refusing to purge under ProgramData reparse point")
	}
	return safeRemoveAllWindows(dir)
}

func validateMSIHelperPurgeTarget(base, dir string) error {
	base = filepath.Clean(strings.TrimSpace(base))
	dir = filepath.Clean(strings.TrimSpace(dir))
	if base == "" || dir == "" {
		return errors.New("refusing to remove empty ProgramData path")
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return err
	}
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if strings.EqualFold(dirAbs, baseAbs) {
		return errors.New("refusing to remove ProgramData root")
	}
	if !strings.EqualFold(filepath.Base(dirAbs), "gov-pass") {
		return errors.New("refusing to remove unexpected directory")
	}
	rel, err := filepath.Rel(baseAbs, dirAbs)
	if err != nil {
		return err
	}
	if rel == "." || rel == "" || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return errors.New("refusing to remove path outside ProgramData")
	}
	if !strings.EqualFold(rel, "gov-pass") {
		return errors.New("refusing to remove unexpected ProgramData child")
	}
	return nil
}

func defaultMSIHelperProgramDataDir() string {
	base, err := msiHelperKnownFolderPath(windows.FOLDERID_ProgramData, windows.KF_FLAG_DEFAULT)
	if err == nil && strings.TrimSpace(base) != "" {
		return filepath.Clean(base)
	}
	return `C:\ProgramData`
}

func stopServiceBestEffort(name string, timeout time.Duration) error {
	m, err := mgr.Connect()
	if err != nil {
		return nil
	}
	defer func() { _ = m.Disconnect() }()

	s, err := m.OpenService(name)
	if err != nil {
		return nil
	}
	defer func() { _ = s.Close() }()

	st, err := s.Query()
	if err == nil && st.State == svc.Stopped {
		return nil
	}

	_, _ = s.Control(svc.Stop)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st, err = s.Query()
		if err != nil {
			return nil
		}
		if st.State == svc.Stopped {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil
}

func deleteServiceBestEffort(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return nil
	}
	defer func() { _ = m.Disconnect() }()

	s, err := m.OpenService(name)
	if err != nil {
		return nil
	}
	defer func() { _ = s.Close() }()

	_ = s.Delete()
	return nil
}

func verifyWinDivert(dir, sysName, serviceName string) error {
	if dir == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve helper path failed: %w", err)
		}
		dir = filepath.Dir(exe)
	}
	report, err := driver.Inspect(context.Background(), driver.Config{
		Dir:         dir,
		SysName:     sysName,
		ServiceName: serviceName,
	})
	if err != nil {
		return fmt.Errorf("inspect WinDivert failed: %w", err)
	}
	if err := printReport(report); err != nil {
		return err
	}
	if report.Healthy() {
		return nil
	}
	return fmt.Errorf("WinDivert verification failed: %s", strings.Join(report.Issues(), "; "))
}

func printReport(report driver.Report) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return fmt.Errorf("encode report failed: %w", err)
	}
	return nil
}
