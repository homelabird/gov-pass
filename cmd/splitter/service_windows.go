//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
)

type serviceRunner func(ctx context.Context, reload <-chan struct{}) error

const (
	defaultServiceLogMaxBytes int64 = 10 * 1024 * 1024
	defaultServiceLogMaxFiles       = 5
	maxServiceLogMaxBytes     int64 = 1024 * 1024 * 1024
	maxServiceLogMaxFiles           = 32
)

type serviceLogConfig struct {
	Path     string
	MaxBytes int64
	MaxFiles int
}

var ensureSecureWindowsServiceLogDir = ensureSecureWindowsDir

func isWindowsServiceProcess() bool {
	ok, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return ok
}

func runService(name string, logCfg serviceLogConfig, run serviceRunner) error {
	if name == "" {
		return errors.New("service-name is empty")
	}

	logFile, err := setupServiceLogging(logCfg)
	if err != nil {
		return err
	}
	if logFile != nil {
		defer func() {
			_ = logFile.Close()
		}()
	}

	isInteractive, err := svc.IsAnInteractiveSession()
	if err != nil {
		return fmt.Errorf("svc interactive session check failed: %w", err)
	}

	handler := &splitterService{run: run}
	if isInteractive {
		return debug.Run(name, handler)
	}
	return svc.Run(name, handler)
}

func setupServiceLogging(cfg serviceLogConfig) (*os.File, error) {
	path := cfg.Path
	if path == "" {
		path = defaultServiceLogPath()
	}
	if err := validateWindowsServiceLogPath(path); err != nil {
		return nil, err
	}

	maxBytes, maxFiles, err := effectiveServiceLogLimits(cfg)
	if err != nil {
		return nil, err
	}

	// Service runs as LocalSystem. Lock down ProgramData state to prevent
	// unprivileged users from tampering with config/log/driver files.
	programDataRoot := filepath.Join(defaultProgramDataDir(), "gov-pass")
	managedProgramDataPath := validateManagedWindowsPath(path, programDataRoot) == nil
	if managedProgramDataPath {
		if err := ensureSecureWindowsDir(programDataRoot); err != nil {
			return nil, fmt.Errorf("secure ProgramData dir failed: %w", err)
		}
	}

	dir := filepath.Dir(path)
	if err := prepareWindowsServiceLogDir(dir, managedProgramDataPath); err != nil {
		return nil, err
	}
	if err := rotateServiceLog(path, maxBytes, maxFiles); err != nil {
		return nil, fmt.Errorf("rotate service log failed: %w", err)
	}
	f, err := openServiceLogFile(path)
	if err != nil {
		return nil, fmt.Errorf("open log file failed: %w", err)
	}
	if managedProgramDataPath {
		if err := hardenWindowsFileACL(path); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("secure log file failed: %w", err)
		}
	}
	log.SetOutput(f)
	log.SetFlags(log.LstdFlags | log.LUTC)
	logInfo("service_log_ready", "service logging configured", "path", path, "max_bytes", maxBytes, "max_files", maxFiles)
	return f, nil
}

func prepareWindowsServiceLogDir(dir string, managedProgramDataPath bool) error {
	if managedProgramDataPath {
		if err := ensureSecureWindowsServiceLogDir(dir); err != nil {
			return fmt.Errorf("secure log dir failed: %w", err)
		}
		return nil
	}
	if err := rejectWindowsReparsePath(dir); err != nil {
		return fmt.Errorf("create log dir failed: %w", err)
	}
	if err := windowsMkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create log dir failed: %w", err)
	}
	if err := rejectWindowsReparsePath(dir); err != nil {
		return fmt.Errorf("create log dir failed: %w", err)
	}
	return nil
}

func effectiveServiceLogLimits(cfg serviceLogConfig) (int64, int, error) {
	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultServiceLogMaxBytes
	}
	if maxBytes > maxServiceLogMaxBytes {
		return 0, 0, fmt.Errorf("service log max bytes must be <= %d", maxServiceLogMaxBytes)
	}

	maxFiles := cfg.MaxFiles
	if maxFiles < 1 {
		maxFiles = defaultServiceLogMaxFiles
	}
	if maxFiles > maxServiceLogMaxFiles {
		return 0, 0, fmt.Errorf("service log max files must be <= %d", maxServiceLogMaxFiles)
	}
	return maxBytes, maxFiles, nil
}

func rotateServiceLog(path string, maxBytes int64, maxFiles int) error {
	if maxBytes <= 0 || maxFiles < 1 {
		return nil
	}

	info, err := lstatServiceLogFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.Size() < maxBytes {
		return nil
	}

	oldest := fmt.Sprintf("%s.%d", path, maxFiles)
	if err := removeRotatedServiceLog(oldest); err != nil {
		return err
	}

	for i := maxFiles - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", path, i)
		dst := fmt.Sprintf("%s.%d", path, i+1)
		if _, err := lstatServiceLogFile(src); err == nil {
			if err := rejectServiceLogReparsePoint(dst); err != nil {
				return err
			}
			if err := os.Rename(src, dst); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	dst := path + ".1"
	if err := rejectServiceLogReparsePoint(dst); err != nil {
		return err
	}
	return os.Rename(path, dst)
}

func lstatServiceLogFile(path string) (os.FileInfo, error) {
	if err := rejectServiceLogReparsePoint(path); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("service log path is not a regular file: %s", path)
	}
	return info, nil
}

func rejectServiceLogReparsePoint(path string) error {
	unsafe, err := windowsPathHasReparsePoint(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if unsafe {
		return fmt.Errorf("refusing service log reparse point: %s", path)
	}
	return nil
}

func openServiceLogFile(path string) (*os.File, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}

	handle, err := windows.CreateFile(
		pathPtr,
		windows.FILE_APPEND_DATA|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}

	var tagInfo windowsFileAttributeTagInfo
	err = windows.GetFileInformationByHandleEx(
		handle,
		windows.FileAttributeTagInfo,
		(*byte)(unsafe.Pointer(&tagInfo)),
		uint32(unsafe.Sizeof(tagInfo)),
	)
	if err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	if tagInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("refusing service log reparse point: %s", path)
	}

	f := os.NewFile(uintptr(handle), path)
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, fmt.Errorf("service log path is not a regular file: %s", path)
	}
	return f, nil
}

func removeRotatedServiceLog(path string) error {
	if err := rejectServiceLogReparsePoint(path); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func defaultServiceLogPath() string {
	return filepath.Join(defaultProgramDataDir(), "gov-pass", "splitter.log")
}

type splitterService struct {
	run serviceRunner
}

func (s *splitterService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptParamChange

	changes <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	reloadCh := make(chan struct{}, 1)
	errCh := make(chan error, 1)
	go func() {
		if s.run == nil {
			errCh <- errors.New("service run func is nil")
			return
		}
		errCh <- s.run(ctx, reloadCh)
	}()

	changes <- svc.Status{State: svc.Running, Accepts: accepted}

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.ParamChange:
				logInfo("service_reload_requested", "service reload requested (paramchange)")
				select {
				case reloadCh <- struct{}{}:
				default:
				}
			case svc.Stop, svc.Shutdown:
				logInfo("service_stop_requested", "service stop requested", "command", c.Cmd)
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				err := <-errCh
				if err != nil && !errors.Is(err, context.Canceled) {
					logError("service_stopped_error", "service stopped with error", err)
					changes <- svc.Status{State: svc.Stopped}
					return false, 1
				}
				logInfo("service_stopped", "service stopped")
				changes <- svc.Status{State: svc.Stopped}
				return false, 0
			default:
				// ignore unsupported commands
			}
		case err := <-errCh:
			cancel()
			if err != nil && !errors.Is(err, context.Canceled) {
				logError("service_exited_error", "service exited with error", err)
				changes <- svc.Status{State: svc.Stopped}
				return false, 1
			}
			logInfo("service_exited", "service exited")
			changes <- svc.Status{State: svc.Stopped}
			return false, 0
		}
	}
}
