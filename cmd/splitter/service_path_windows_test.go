//go:build windows

package main

import "testing"

func TestValidateWindowsServiceConfigPath(t *testing.T) {
	t.Setenv("ProgramData", `C:\ProgramData`)

	if err := validateWindowsServiceConfigPath(`C:\ProgramData\gov-pass\config.json`); err != nil {
		t.Fatalf("expected ProgramData config path to pass: %v", err)
	}
	if err := validateWindowsServiceConfigPath(`config.json`); err == nil {
		t.Fatal("expected relative config path to fail")
	}
	if err := validateWindowsServiceConfigPath(`C:\Temp\config.json`); err == nil {
		t.Fatal("expected config path outside ProgramData to fail")
	}
}

func TestValidateWindowsServiceLogPath(t *testing.T) {
	t.Setenv("ProgramData", `C:\ProgramData`)

	if err := validateWindowsServiceLogPath(`C:\ProgramData\gov-pass\splitter.log`); err != nil {
		t.Fatalf("expected ProgramData log path to pass: %v", err)
	}
	if err := validateWindowsServiceLogPath(`C:\Logs\splitter.log`); err == nil {
		t.Fatal("expected log path outside ProgramData to fail")
	}
}

func TestValidateWindowsServiceDriverDir(t *testing.T) {
	t.Setenv("ProgramData", `C:\ProgramData`)
	t.Setenv("ProgramFiles", `C:\Program Files`)

	if err := validateWindowsServiceDriverDir(`C:\Program Files\gov-pass`); err != nil {
		t.Fatalf("expected install-root driver dir to pass: %v", err)
	}
	if err := validateWindowsServiceDriverDir(`C:\ProgramData\gov-pass\windivert`); err != nil {
		t.Fatalf("expected ProgramData driver dir to pass: %v", err)
	}
	if err := validateWindowsServiceDriverDir(`C:\Temp\windivert`); err == nil {
		t.Fatal("expected driver dir outside managed roots to fail")
	}
}
