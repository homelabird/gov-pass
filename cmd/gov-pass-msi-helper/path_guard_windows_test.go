//go:build windows

package main

import "testing"

func TestValidateMSIHelperPurgeTarget(t *testing.T) {
	if err := validateMSIHelperPurgeTarget(`C:\ProgramData`, `C:\ProgramData\gov-pass`); err != nil {
		t.Fatalf("expected gov-pass child to pass: %v", err)
	}
	if err := validateMSIHelperPurgeTarget(`C:\ProgramData`, `C:\ProgramData`); err == nil {
		t.Fatal("expected ProgramData root to fail")
	}
	if err := validateMSIHelperPurgeTarget(`C:\ProgramData`, `C:\ProgramData\other`); err == nil {
		t.Fatal("expected unexpected child to fail")
	}
	if err := validateMSIHelperPurgeTarget(`C:\ProgramData`, `C:\Temp\gov-pass`); err == nil {
		t.Fatal("expected outside ProgramData path to fail")
	}
}
