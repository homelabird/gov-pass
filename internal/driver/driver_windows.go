//go:build windows

package driver

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

var ErrAdminRequired = errors.New("administrator privileges required")
var ErrDriverNotFound = errors.New("WinDivert driver sys not found")

var (
	shell32           = syscall.NewLazyDLL("shell32.dll")
	procIsUserAnAdmin = shell32.NewProc("IsUserAnAdmin")
)

// Ensure installs and starts the WinDivert driver if needed.
// It returns a cleanup function that will stop/delete the driver based on config.
func Ensure(ctx context.Context, cfg Config) (func() error, error) {
	_, cleanup, err := EnsureWithReport(ctx, cfg)
	return cleanup, err
}

func EnsureWithReport(ctx context.Context, cfg Config) (Report, func() error, error) {
	report, err := Inspect(ctx, cfg)
	if err != nil {
		return report, nil, err
	}
	if _, err := os.Stat(report.ResolvedSysPath); err != nil {
		return report, nil, ErrDriverNotFound
	}
	if !cfg.AutoInstall {
		return report, nil, nil
	}

	created := false
	started := false

	if !report.ServiceExists {
		if !isAdmin() {
			return report, nil, ErrAdminRequired
		}
		if err := createService(ctx, report.ServiceName, report.ResolvedSysPath); err != nil {
			return report, nil, err
		}
		created = true
		report.ServiceExists = true
		report.ServiceCreated = true
		report.ServiceBinPath = report.ResolvedSysPath
		report.ServiceBinPathExists = true
		report.ServiceBinPathMatchesDesired = true
		report.ServiceNeedsConfigRepair = false
	} else {
		if report.ServiceNeedsConfigRepair {
			if !isAdmin() {
				return report, nil, ErrAdminRequired
			}
			if err := configService(ctx, report.ServiceName, report.ResolvedSysPath); err != nil {
				return report, nil, err
			}
			report.ServiceReconfigured = true
			report.ServiceBinPath = report.ResolvedSysPath
			report.ServiceBinPathExists = true
			report.ServiceBinPathMatchesDesired = true
			report.ServiceNeedsConfigRepair = false
		}
	}

	if !report.ServiceRunning {
		if !isAdmin() {
			return report, nil, ErrAdminRequired
		}
		if err := startService(ctx, report.ServiceName); err != nil {
			return report, nil, err
		}
		started = true
		report.ServiceRunning = true
		report.ServiceStarted = true
	}

	cleanup := func() error {
		if created && cfg.AutoUninstall {
			if err := stopService(ctx, report.ServiceName); err != nil {
				return err
			}
			if err := deleteService(ctx, report.ServiceName); err != nil {
				return err
			}
			return nil
		}
		if started && cfg.AutoStop {
			if err := stopService(ctx, report.ServiceName); err != nil {
				return err
			}
		}
		return nil
	}
	report.CleanupWillDelete = created && cfg.AutoUninstall
	report.CleanupWillStop = started && cfg.AutoStop && !report.CleanupWillDelete
	return report, cleanup, nil
}

func Inspect(ctx context.Context, cfg Config) (Report, error) {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "WinDivert"
	}
	if err := ValidateServiceName(cfg.ServiceName); err != nil {
		return Report{}, err
	}
	if err := ValidateDriverFileName(cfg.SysName); err != nil {
		return Report{}, err
	}

	dir, err := resolveDir(cfg.Dir)
	if err != nil {
		return Report{}, err
	}

	sysPath, sysExists := probeSysPath(dir, cfg.SysName)
	if sysPath == "" {
		sysPath = filepath.Join(dir, effectiveSysName(cfg.SysName))
	}

	exists, running, err := queryService(ctx, cfg.ServiceName)
	if err != nil {
		return Report{}, err
	}

	report := Report{
		ResolvedDir:     dir,
		ResolvedSysPath: sysPath,
		ServiceName:     cfg.ServiceName,
		FilesPresent:    HasWinDivertFiles(dir, cfg.SysName),
		ServiceExists:   exists,
		ServiceRunning:  running,
	}
	if !sysExists {
		return report, nil
	}
	if !exists {
		return report, nil
	}

	path, err := queryServiceBinPath(ctx, cfg.ServiceName)
	if err != nil {
		return report, err
	}
	report.ServiceBinPath = path
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			report.ServiceBinPathExists = true
		}
		report.ServiceBinPathMatchesDesired = sameServicePath(path, sysPath)
	}
	report.ServiceNeedsConfigRepair = path == "" || !report.ServiceBinPathExists || !report.ServiceBinPathMatchesDesired
	return report, nil
}

func isAdmin() bool {
	ret, _, _ := procIsUserAnAdmin.Call()
	return ret != 0
}

func resolveDir(dir string) (string, error) {
	if dir == "" {
		exe, err := os.Executable()
		if err != nil {
			return "", err
		}
		dir = filepath.Dir(exe)
	}
	if _, err := os.Stat(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func resolveSysPath(dir, name string) (string, error) {
	if path, ok := probeSysPath(dir, name); ok {
		return path, nil
	}
	return "", ErrDriverNotFound
}

func probeSysPath(dir, name string) (string, bool) {
	if name != "" {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
		return path, false
	}

	candidates := []string{"WinDivert64.sys", "WinDivert.sys"}
	for _, candidate := range candidates {
		path := filepath.Join(dir, candidate)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

func effectiveSysName(name string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	return "WinDivert64.sys"
}

func queryService(ctx context.Context, name string) (bool, bool, error) {
	out, err := runSC(ctx, "query", name)
	if err != nil {
		if isServiceMissing(out) {
			return false, false, nil
		}
		return false, false, err
	}
	running := strings.Contains(strings.ToUpper(out), "RUNNING")
	return true, running, nil
}

func createService(ctx context.Context, name, sysPath string) error {
	_, err := runSC(ctx, "create", name, "type=", "kernel", "start=", "demand", "binPath=", sysPath)
	return err
}

func configService(ctx context.Context, name, sysPath string) error {
	_, err := runSC(ctx, "config", name, "start=", "demand", "binPath=", sysPath)
	return err
}

func startService(ctx context.Context, name string) error {
	_, err := runSC(ctx, "start", name)
	return err
}

func stopService(ctx context.Context, name string) error {
	out, err := runSC(ctx, "stop", name)
	if err != nil && isServiceNotRunning(out) {
		return nil
	}
	return err
}

func deleteService(ctx context.Context, name string) error {
	_, err := runSC(ctx, "delete", name)
	if err != nil {
		return err
	}
	return nil
}

func runSC(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "sc.exe", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func queryServiceBinPath(ctx context.Context, name string) (string, error) {
	out, err := runSC(ctx, "qc", name)
	if err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "BINARY_PATH_NAME") {
			if idx := strings.Index(line, ":"); idx != -1 {
				raw := strings.TrimSpace(line[idx+1:])
				return normalizeServicePath(raw), nil
			}
		}
	}
	return "", scanner.Err()
}

func normalizeServicePath(raw string) string {
	path := strings.TrimSpace(raw)
	path = strings.Trim(path, "\"")
	if strings.HasPrefix(path, "\\??\\") {
		path = path[4:]
	}
	lower := strings.ToLower(path)
	if idx := strings.Index(lower, ".sys"); idx != -1 {
		return path[:idx+4]
	}
	if idx := strings.Index(path, " "); idx != -1 {
		return path[:idx]
	}
	return path
}

func sameServicePath(a, b string) bool {
	a = filepath.Clean(strings.TrimSpace(a))
	b = filepath.Clean(strings.TrimSpace(b))
	return strings.EqualFold(a, b)
}

func isServiceMissing(out string) bool {
	s := strings.ToLower(out)
	return strings.Contains(s, "1060") || strings.Contains(s, "does not exist")
}

func isServiceNotRunning(out string) bool {
	s := strings.ToLower(out)
	return strings.Contains(s, "1062") || strings.Contains(s, "not been started")
}
