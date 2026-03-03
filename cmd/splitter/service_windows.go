//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
)

type serviceRunner func(ctx context.Context, reload <-chan struct{}) error

const (
	defaultServiceLogMaxBytes int64 = 10 * 1024 * 1024
	defaultServiceLogMaxFiles       = 5
)

type serviceLogConfig struct {
	Path     string
	MaxBytes int64
	MaxFiles int
}

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

	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultServiceLogMaxBytes
	}

	maxFiles := cfg.MaxFiles
	if maxFiles < 1 {
		maxFiles = defaultServiceLogMaxFiles
	}

	// Service runs as LocalSystem. Lock down ProgramData state to prevent
	// unprivileged users from tampering with config/log/driver files.
	programDataRoot := filepath.Join(defaultProgramDataDir(), "gov-pass")
	if isUnderDir(path, programDataRoot) {
		if err := ensureSecureWindowsDir(programDataRoot); err != nil {
			return nil, fmt.Errorf("secure ProgramData dir failed: %w", err)
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir failed: %w", err)
	}
	if err := rotateServiceLog(path, maxBytes, maxFiles); err != nil {
		return nil, fmt.Errorf("rotate service log failed: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file failed: %w", err)
	}
	if isUnderDir(path, programDataRoot) {
		if err := hardenWindowsFileACL(path); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("secure log file failed: %w", err)
		}
	}
	log.SetOutput(f)
	log.SetFlags(log.LstdFlags | log.LUTC)
	log.Printf("service logging to %s (max_bytes=%d max_files=%d)", path, maxBytes, maxFiles)
	return f, nil
}

func rotateServiceLog(path string, maxBytes int64, maxFiles int) error {
	if maxBytes <= 0 || maxFiles < 1 {
		return nil
	}

	info, err := os.Stat(path)
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
	_ = os.Remove(oldest)

	for i := maxFiles - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", path, i)
		dst := fmt.Sprintf("%s.%d", path, i+1)
		if _, err := os.Stat(src); err == nil {
			if err := os.Rename(src, dst); err != nil {
				return err
			}
		}
	}

	return os.Rename(path, path+".1")
}

func defaultServiceLogPath() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "gov-pass", "splitter.log")
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
				log.Printf("service reload requested (paramchange)")
				select {
				case reloadCh <- struct{}{}:
				default:
				}
			case svc.Stop, svc.Shutdown:
				log.Printf("service stop requested (%v)", c.Cmd)
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				err := <-errCh
				if err != nil && !errors.Is(err, context.Canceled) {
					log.Printf("service stopped with error: %v", err)
					changes <- svc.Status{State: svc.Stopped}
					return false, 1
				}
				log.Printf("service stopped")
				changes <- svc.Status{State: svc.Stopped}
				return false, 0
			default:
				// ignore unsupported commands
			}
		case err := <-errCh:
			cancel()
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("service exited with error: %v", err)
				changes <- svc.Status{State: svc.Stopped}
				return false, 1
			}
			log.Printf("service exited")
			changes <- svc.Status{State: svc.Stopped}
			return false, 0
		}
	}
}
