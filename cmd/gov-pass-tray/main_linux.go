//go:build linux

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/getlantern/systray"
)

const (
	defaultServiceName = "gov-pass"
)

func main() {
	serviceName := flag.String("service-name", defaultServiceName, "systemd service name to control")
	action := flag.String("action", "", "action mode: start|stop|restart|toggle|status (runs and exits; may prompt for elevation)")
	flag.Parse()

	name := strings.TrimSpace(*serviceName)
	if name == "" {
		name = defaultServiceName
	}

	act := strings.ToLower(strings.TrimSpace(*action))
	if act != "" {
		if err := runAction(name, act); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	ui := &trayUI{serviceName: name}
	systray.Run(ui.onReady, ui.onExit)
}

type trayUI struct {
	serviceName string

	mStatus  *systray.MenuItem
	mToggle  *systray.MenuItem
	mRestart *systray.MenuItem
	mQuit    *systray.MenuItem

	iconOn  []byte
	iconOff []byte
	iconErr []byte
}

func (t *trayUI) onReady() {
	t.iconOn = mustBuildIcoCircle(32, rgba{0x34, 0xc7, 0x59, 0xff})  // Apple system green
	t.iconOff = mustBuildIcoCircle(32, rgba{0x8e, 0x8e, 0x93, 0xff}) // Apple system gray
	t.iconErr = mustBuildIcoCircle(32, rgba{0xff, 0x3b, 0x30, 0xff}) // Apple system red

	systray.SetIcon(t.iconOff)
	systray.SetTooltip("gov-pass")

	t.mStatus = systray.AddMenuItem("Status: Checking…", "")
	t.mStatus.Disable()
	systray.AddSeparator()

	t.mToggle = systray.AddMenuItem("Activate Protection…", "Toggle protection on or off")
	t.mRestart = systray.AddMenuItem("Restart Service…", "Restart the background service")
	systray.AddSeparator()

	t.mQuit = systray.AddMenuItem("Quit gov-pass", "Quit the application")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer cancel()
		t.pollStatus(ctx)
	}()

	go func() {
		for {
			select {
			case <-t.mToggle.ClickedCh:
				go t.toggleService()
			case <-t.mRestart.ClickedCh:
				go t.restartService()
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

func (t *trayUI) toggleService() {
	active, err := isServiceActive(t.serviceName)
	if err == nil && active {
		if err := elevatedSystemctl("stop", t.serviceName); err != nil {
			log.Printf("stop service failed: %v", err)
		}
		return
	}
	if err := elevatedSystemctl("start", t.serviceName); err != nil {
		log.Printf("start service failed: %v", err)
	}
}

func (t *trayUI) restartService() {
	if err := elevatedSystemctl("restart", t.serviceName); err != nil {
		log.Printf("restart service failed: %v", err)
	}
}

func (t *trayUI) pollStatus(ctx context.Context) {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	var lastActive *bool
	var lastErr bool

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			active, err := isServiceActive(t.serviceName)
			if err != nil {
				if !lastErr {
					systray.SetIcon(t.iconErr)
					systray.SetTooltip("gov-pass — Status Unknown")
					t.mStatus.SetTitle("Status: Unknown")
					t.mToggle.SetTitle("Start Service…")
					t.mRestart.Disable()
					lastErr = true
					lastActive = nil
				}
				continue
			}
			lastErr = false

			if lastActive == nil || *lastActive != active {
				lastActive = &active

				if active {
					systray.SetIcon(t.iconOn)
					systray.SetTooltip("gov-pass — Active")
					t.mStatus.SetTitle("● Status: Active")
					t.mToggle.SetTitle("Deactivate Protection…")
					t.mRestart.Enable()
				} else {
					systray.SetIcon(t.iconOff)
					systray.SetTooltip("gov-pass — Stopped")
					t.mStatus.SetTitle("○ Status: Inactive")
					t.mToggle.SetTitle("Activate Protection…")
					t.mRestart.Enable()
				}
			}
		}
	}
}

func runAction(serviceName string, action string) error {
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case "start", "stop", "restart", "toggle", "status":
	default:
		return fmt.Errorf("unknown action: %s", action)
	}

	switch action {
	case "status":
		active, err := isServiceActive(serviceName)
		if err != nil {
			return err
		}
		if active {
			fmt.Println("active")
		} else {
			fmt.Println("inactive")
		}
		return nil
	case "toggle":
		active, err := isServiceActive(serviceName)
		if err != nil {
			return err
		}
		if active {
			return elevatedSystemctl("stop", serviceName)
		}
		return elevatedSystemctl("start", serviceName)
	default:
		return elevatedSystemctl(action, serviceName)
	}
}

// isServiceActive checks whether a systemd service is running.
func isServiceActive(serviceName string) (bool, error) {
	out, err := exec.Command("systemctl", "is-active", serviceName).CombinedOutput()
	state := strings.TrimSpace(string(out))
	if state == "active" {
		return true, nil
	}
	if state == "inactive" || state == "failed" || state == "dead" {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("systemctl is-active %s: %w (%s)", serviceName, err, state)
	}
	return false, nil
}

// elevatedSystemctl runs a systemctl command with privilege elevation via pkexec.
func elevatedSystemctl(action string, serviceName string) error {
	cmd := exec.Command("pkexec", "systemctl", action, serviceName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
