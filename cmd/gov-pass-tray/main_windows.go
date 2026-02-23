//go:build windows

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
	"unsafe"

	"github.com/getlantern/systray"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	defaultServiceName = "gov-pass"

	runKeyPath  = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueKey = "gov-pass-tray"
)

func main() {
	serviceName := flag.String("service-name", defaultServiceName, "Windows service name to control")
	action := flag.String("action", "", "action mode: start|stop|restart|reload|toggle|status (runs and exits; may prompt for elevation)")
	flag.Parse()

	name := strings.TrimSpace(*serviceName)
	if name == "" {
		name = defaultServiceName
	}

	act := strings.ToLower(strings.TrimSpace(*action))
	if act != "" {
		if err := runAction(name, act); err != nil {
			// In windowsgui mode there may be no console; keep a non-zero exit code.
			os.Exit(1)
		}
		return
	}

	ui := &trayUI{serviceName: name}
	systray.Run(ui.onReady, ui.onExit)
}

type trayUI struct {
	serviceName string

	mDashboard *systray.MenuItem
	mStatus    *systray.MenuItem
	mToggle    *systray.MenuItem
	mReload    *systray.MenuItem
	mRestart   *systray.MenuItem
	mRunAtLog  *systray.MenuItem
	mQuit      *systray.MenuItem

	iconOn  []byte
	iconOff []byte
	iconErr []byte
}

func (t *trayUI) onReady() {
	t.iconOn = mustBuildIcoCircle(32, rgba{0x2e, 0xcc, 0x71, 0xff})  // green
	t.iconOff = mustBuildIcoCircle(32, rgba{0x95, 0xa5, 0xa6, 0xff}) // gray
	t.iconErr = mustBuildIcoCircle(32, rgba{0xe7, 0x4c, 0x3c, 0xff}) // red

	systray.SetIcon(t.iconOff)
	systray.SetTooltip("gov-pass")

	t.mDashboard = systray.AddMenuItem("🎨 Dashboard", "")
	systray.AddSeparator()

	t.mStatus = systray.AddMenuItem("Status: ...", "")
	t.mStatus.Disable()

	systray.AddSeparator()

	t.mToggle = systray.AddMenuItem("Activate Protection (Admin)...", "")
	t.mReload = systray.AddMenuItem("Reload config (Admin)...", "")
	t.mRestart = systray.AddMenuItem("Restart service (Admin)...", "")
	t.mReload.Disable()

	systray.AddSeparator()

	enabled, _ := runAtLoginEnabled()
	t.mRunAtLog = systray.AddMenuItemCheckbox("Start tray at login", "", enabled)
	if enabled {
		t.mRunAtLog.Check()
	} else {
		t.mRunAtLog.Uncheck()
	}

	systray.AddSeparator()
	t.mQuit = systray.AddMenuItem("Quit", "")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer cancel()
		t.pollStatus(ctx)
	}()

	go func() {
		for {
			select {
			case <-t.mDashboard.ClickedCh:
				go t.showDashboard()
			case <-t.mToggle.ClickedCh:
				t.elevateToggle()
			case <-t.mReload.ClickedCh:
				_ = elevateSelf(t.serviceName, "reload")
			case <-t.mRestart.ClickedCh:
				_ = elevateSelf(t.serviceName, "restart")
			case <-t.mRunAtLog.ClickedCh:
				t.toggleRunAtLogin()
			case <-t.mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func (t *trayUI) onExit() {
	// nothing to cleanup
}

func (t *trayUI) elevateToggle() {
	state, err := queryServiceState(t.serviceName)
	if err == nil && state == svc.Running {
		_ = elevateSelf(t.serviceName, "stop")
		return
	}
	_ = elevateSelf(t.serviceName, "start")
}

func (t *trayUI) showDashboard() {
	state, err := queryServiceState(t.serviceName)
	stStr := "Unknown"
	if err == nil {
		switch state {
		case svc.Running:
			stStr = "ACTIVE (Running)"
		case svc.Stopped:
			stStr = "INACTIVE (Stopped)"
		default:
			s := stateString(state)
			if len(s) > 0 {
				stStr = strings.ToUpper(s[:1]) + s[1:]
			} else {
				stStr = s
			}
		}
	}

	titlePtr, _ := windows.UTF16PtrFromString("gov-pass Dashboard")
	textPtr, _ := windows.UTF16PtrFromString(fmt.Sprintf(
		"gov-pass Splitter\n"+
			"------------------\n\n"+
			"Current Status: %s\n\n"+
			"The splitter is used to bypass network restrictions by splitting TLS ClientHellos.\n\n"+
			"Would you like to toggle the protection state?", stStr))

	// MB_YESNO | MB_ICONQUESTION | MB_TOPMOST
	// Note: windows.IDYES is often not defined in x/sys/windows; using literal 6.
	ret, _ := windows.MessageBox(0, textPtr, titlePtr, windows.MB_YESNO|windows.MB_ICONQUESTION|windows.MB_TOPMOST)
	if ret == 6 {
		t.elevateToggle()
	}
}

func (t *trayUI) toggleRunAtLogin() {
	enabled, _ := runAtLoginEnabled()
	next := !enabled
	if err := setRunAtLogin(next); err != nil {
		// Best-effort UI feedback: just revert the check state.
		if enabled {
			t.mRunAtLog.Check()
		} else {
			t.mRunAtLog.Uncheck()
		}
		return
	}
	if next {
		t.mRunAtLog.Check()
	} else {
		t.mRunAtLog.Uncheck()
	}
}

func (t *trayUI) pollStatus(ctx context.Context) {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	var lastState svc.State
	var haveLast bool
	var lastErr bool

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			state, err := queryServiceState(t.serviceName)
			if err != nil {
				if !lastErr {
					systray.SetIcon(t.iconErr)
					systray.SetTooltip("gov-pass: status unknown")
					t.mStatus.SetTitle("Status: Unknown (service query failed)")
					t.mToggle.SetTitle("Start service (Admin)...")
					t.mReload.Disable()
					t.mRestart.Disable()
					lastErr = true
				}
				continue
			}
			lastErr = false

			if !haveLast || state != lastState {
				haveLast = true
				lastState = state

				switch state {
				case svc.Running:
					systray.SetIcon(t.iconOn)
					systray.SetTooltip("gov-pass: Running")
					t.mStatus.SetTitle("🟢 Status: Active")
					t.mToggle.SetTitle("Deactivate Protection (Admin)...")
					t.mReload.Enable()
					t.mRestart.Enable()
				case svc.Stopped:
					systray.SetIcon(t.iconOff)
					systray.SetTooltip("gov-pass: Stopped")
					t.mStatus.SetTitle("⚪ Status: Inactive")
					t.mToggle.SetTitle("Activate Protection (Admin)...")
					t.mReload.Disable()
					t.mRestart.Enable()
				case svc.StartPending:
					systray.SetIcon(t.iconOff)
					systray.SetTooltip("gov-pass: starting")
					t.mStatus.SetTitle("Status: Starting...")
					t.mToggle.Disable()
					t.mReload.Disable()
					t.mRestart.Disable()
				case svc.StopPending:
					systray.SetIcon(t.iconOff)
					systray.SetTooltip("gov-pass: stopping")
					t.mStatus.SetTitle("Status: Stopping...")
					t.mToggle.Disable()
					t.mReload.Disable()
					t.mRestart.Disable()
				default:
					systray.SetIcon(t.iconOff)
					systray.SetTooltip("gov-pass: state unknown")
					t.mStatus.SetTitle(fmt.Sprintf("Status: %s", stateString(state)))
					t.mToggle.Enable()
					t.mReload.Disable()
					t.mRestart.Enable()
				}

				// Re-enable toggle after pending states complete.
				if state != svc.StartPending && state != svc.StopPending {
					t.mToggle.Enable()
				}
			}
		}
	}
}

func runAction(serviceName string, action string) error {
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case "start", "stop", "restart", "reload", "toggle", "status":
	default:
		return fmt.Errorf("unknown action: %s", action)
	}

	if !isElevated() && action != "status" {
		return elevateSelf(serviceName, action)
	}

	switch action {
	case "status":
		_, err := queryServiceState(serviceName)
		return err
	case "toggle":
		state, err := queryServiceState(serviceName)
		if err != nil {
			return err
		}
		if state == svc.Running {
			return controlService(serviceName, "stop")
		}
		return controlService(serviceName, "start")
	default:
		return controlService(serviceName, action)
	}
}

func controlService(serviceName string, action string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	switch action {
	case "start":
		if err := s.Start(); err != nil {
			return err
		}
		return waitForServiceState(s, svc.Running, 30*time.Second)
	case "stop":
		if _, err := s.Control(svc.Stop); err != nil {
			return err
		}
		return waitForServiceState(s, svc.Stopped, 30*time.Second)
	case "restart":
		// Stop is best-effort if it's already stopped.
		_, _ = s.Control(svc.Stop)
		_ = waitForServiceState(s, svc.Stopped, 30*time.Second)
		if err := s.Start(); err != nil {
			return err
		}
		return waitForServiceState(s, svc.Running, 30*time.Second)
	case "reload":
		_, err := s.Control(svc.ParamChange)
		return err
	default:
		return fmt.Errorf("unknown service action: %s", action)
	}
}

func waitForServiceState(s *mgr.Service, want svc.State, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st, err := s.Query()
		if err != nil {
			return err
		}
		if st.State == want {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for service state: %s", stateString(want))
}

func queryServiceState(serviceName string) (svc.State, error) {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		return svc.Stopped, errors.New("service name is empty")
	}

	hSCM, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return svc.Stopped, err
	}
	defer func() { _ = windows.CloseServiceHandle(hSCM) }()

	namePtr, err := windows.UTF16PtrFromString(serviceName)
	if err != nil {
		return svc.Stopped, err
	}
	hSvc, err := windows.OpenService(hSCM, namePtr, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return svc.Stopped, err
	}
	defer func() { _ = windows.CloseServiceHandle(hSvc) }()

	var t windows.SERVICE_STATUS_PROCESS
	var needed uint32
	err = windows.QueryServiceStatusEx(hSvc, windows.SC_STATUS_PROCESS_INFO, (*byte)(unsafe.Pointer(&t)), uint32(unsafe.Sizeof(t)), &needed)
	if err != nil {
		return svc.Stopped, err
	}
	return svc.State(t.CurrentState), nil
}

func isElevated() bool {
	var tok windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &tok)
	if err != nil {
		return false
	}
	defer func() { _ = tok.Close() }()
	return tok.IsElevated()
}

func elevateSelf(serviceName string, action string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe = strings.TrimSpace(exe)
	if exe == "" {
		return errors.New("executable path is empty")
	}

	args := []string{
		"--service-name", serviceName,
		"--action", action,
	}
	argStr := quoteArgs(args)

	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	params, _ := windows.UTF16PtrFromString(argStr)
	cwd, _ := windows.UTF16PtrFromString("")

	return windows.ShellExecute(0, verb, file, params, cwd, windows.SW_NORMAL)
}

func quoteArgs(args []string) string {
	var b strings.Builder
	for i := 0; i < len(args); i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(quoteArg(args[i]))
	}
	return b.String()
}

func quoteArg(s string) string {
	// Minimal Windows command line quoting. It is sufficient for our current
	// flag set and avoids pulling in external deps.
	if s == "" {
		return `""`
	}
	need := strings.ContainsAny(s, " \t\"")
	if !need {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, r := range s {
		switch r {
		case '\\':
			backslashes++
		case '"':
			b.WriteString(strings.Repeat("\\", backslashes*2+1))
			b.WriteByte('"')
			backslashes = 0
		default:
			if backslashes > 0 {
				b.WriteString(strings.Repeat("\\", backslashes))
				backslashes = 0
			}
			b.WriteRune(r)
		}
	}
	if backslashes > 0 {
		b.WriteString(strings.Repeat("\\", backslashes*2))
	}
	b.WriteByte('"')
	return b.String()
}

func runAtLoginEnabled() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer func() { _ = k.Close() }()

	_, _, err = k.GetStringValue(runValueKey)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func setRunAtLogin(enabled bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()

	if !enabled {
		err := k.DeleteValue(runValueKey)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe = strings.TrimSpace(exe)
	if exe == "" {
		return errors.New("executable path is empty")
	}
	// Keep this value simple: run tray UI without extra args.
	return k.SetStringValue(runValueKey, quoteArg(exe))
}

func stateString(st svc.State) string {
	switch st {
	case svc.Running:
		return "running"
	case svc.Stopped:
		return "stopped"
	case svc.StartPending:
		return "start-pending"
	case svc.StopPending:
		return "stop-pending"
	default:
		return fmt.Sprintf("state-%d", uint32(st))
	}
}
