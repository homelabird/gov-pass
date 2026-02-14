//go:build windows

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	runKeyPath  = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueKey = "gov-pass-tray"

	trayExeName      = "gov-pass-tray.exe"
	winDivertSvcName = "WinDivert"
)

func main() {
	action := flag.String("action", "", "action: kill-tray|clean-autorun-hkcu|clean-autorun-hklm|purge-programdata|stop-windivert|delete-windivert")
	flag.Parse()

	act := strings.ToLower(strings.TrimSpace(*action))
	switch act {
	case "kill-tray":
		_ = killTrayBestEffort()
	case "clean-autorun-hkcu":
		_ = cleanAutorunBestEffort(registry.CURRENT_USER)
	case "clean-autorun-hklm":
		_ = cleanAutorunBestEffort(registry.LOCAL_MACHINE)
	case "purge-programdata":
		_ = purgeProgramDataBestEffort()
	case "stop-windivert":
		_ = stopServiceBestEffort(winDivertSvcName, 10*time.Second)
	case "delete-windivert":
		_ = deleteServiceBestEffort(winDivertSvcName)
	default:
		// MSI custom actions use Return="ignore", but keep a non-zero exit code
		// for manual invocation/debugging.
		_, _ = fmt.Fprintf(os.Stderr, "unknown --action: %q\n", act)
		os.Exit(2)
	}
}

func killTrayBestEffort() error {
	sysDir, err := windows.GetSystemDirectory()
	taskkill := "taskkill.exe"
	if err == nil && strings.TrimSpace(sysDir) != "" {
		taskkill = filepath.Join(sysDir, taskkill)
	}

	cmd := exec.Command(taskkill, "/IM", trayExeName, "/F")
	// Suppress output: MSI logs would capture this, but we keep it quiet.
	cmd.Stdout = nil
	cmd.Stderr = nil
	_ = cmd.Run()
	return nil
}

func cleanAutorunBestEffort(root registry.Key) error {
	k, err := registry.OpenKey(root, runKeyPath, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return nil
	}
	defer func() { _ = k.Close() }()

	err = k.DeleteValue(runValueKey)
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return nil
}

func purgeProgramDataBestEffort() error {
	base := strings.TrimSpace(os.Getenv("ProgramData"))
	if base == "" {
		base = `C:\ProgramData`
	}
	dir := filepath.Join(base, "gov-pass")

	// Safety guard: never remove the entire ProgramData root.
	if strings.EqualFold(filepath.Clean(dir), filepath.Clean(base)) {
		return errors.New("refusing to remove ProgramData root")
	}
	if !strings.EqualFold(filepath.Base(dir), "gov-pass") {
		return errors.New("refusing to remove unexpected directory")
	}

	_ = os.RemoveAll(dir)
	return nil
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
