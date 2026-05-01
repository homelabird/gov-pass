package packaging_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsPrivilegedCommandsAvoidEnvironmentFallback(t *testing.T) {
	root := repoRoot(t)
	files := []string{
		filepath.Join("cmd", "gov-pass-msi-helper", "main_windows.go"),
		filepath.Join("cmd", "gov-pass-tui", "windows_command_common.go"),
		filepath.Join("cmd", "splitter", "acl_windows.go"),
		filepath.Join("internal", "driver", "driver_windows.go"),
	}

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, file))
			for _, forbidden := range []string{
				`os.Getenv("SystemRoot")`,
				`os.Getenv("windir")`,
				`exec.LookPath`,
				"`C:\\Windows`",
			} {
				if strings.Contains(text, forbidden) {
					t.Fatalf("%s uses environment-based System32 fallback %q", file, forbidden)
				}
			}
		})
	}
}

func TestMSIHelperTaskkillUsesSystemDirectory(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "cmd", "gov-pass-msi-helper", "main_windows.go"))
	for _, fragment := range []string{
		`msiHelperSystemDirectory = windows.GetSystemDirectory`,
		`taskkill, err := resolveMSIHelperSystem32Command("taskkill.exe")`,
		`sysDir, err := msiHelperSystemDirectory()`,
		`return "", fmt.Errorf("resolve System32 failed: %w", err)`,
		`path := filepath.Join(sysDir, name)`,
		`windowsPathHasReparsePoint(path)`,
		`System32 command must not be a reparse point`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("MSI helper taskkill path is missing required fragment %q", fragment)
		}
	}
	if strings.Contains(text, `taskkill := "taskkill.exe"`) {
		t.Fatal("MSI helper still allows PATH-based taskkill fallback")
	}
}

func TestWindowsTUICommandsRejectReparsePoints(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "cmd", "gov-pass-tui", "windows_command_common.go"))
	for _, fragment := range []string{
		`windowsCommandGetFileAttributes = windows.GetFileAttributes`,
		`isSafeExecutableWindowsCommandFile(candidate)`,
		`windowsCommandPathHasReparsePoint(path)`,
		`windows.FILE_ATTRIBUTE_REPARSE_POINT`,
		`Windows command must not be a reparse point`,
		`command name must be a bare filename`,
		`errors.Is(err, os.ErrNotExist)`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("cmd/gov-pass-tui/windows_command_common.go does not contain required command reparse guard %q", fragment)
		}
	}
}

func TestWindowsDefaultsPreferKnownFolders(t *testing.T) {
	root := repoRoot(t)
	tests := []struct {
		name      string
		path      string
		fragments []string
		forbidden []string
	}{
		{
			name: "splitter service defaults",
			path: filepath.Join("cmd", "splitter", "config_windows.go"),
			fragments: []string{
				`windowsKnownFolderPath = windows.KnownFolderPath`,
				`windows.FOLDERID_ProgramData`,
				`windows.FOLDERID_ProgramFiles`,
			},
			forbidden: []string{
				`os.Getenv("ProgramData")`,
				`os.Getenv("ProgramFiles")`,
			},
		},
		{
			name: "splitter service log",
			path: filepath.Join("cmd", "splitter", "service_windows.go"),
			fragments: []string{
				`filepath.Join(defaultProgramDataDir(), "gov-pass", "splitter.log")`,
			},
			forbidden: []string{
				`os.Getenv("ProgramData")`,
			},
		},
		{
			name: "msi helper purge",
			path: filepath.Join("cmd", "gov-pass-msi-helper", "main_windows.go"),
			fragments: []string{
				`msiHelperKnownFolderPath = windows.KnownFolderPath`,
				`msiHelperKnownFolderPath(windows.FOLDERID_ProgramData, windows.KF_FLAG_DEFAULT)`,
				`defaultMSIHelperProgramDataDir()`,
				`validateMSIHelperPurgeTarget(base, dir)`,
			},
			forbidden: []string{
				`os.Getenv("ProgramData")`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, tt.path))
			for _, fragment := range tt.fragments {
				if !strings.Contains(text, fragment) {
					t.Fatalf("%s missing required fragment %q", tt.path, fragment)
				}
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(text, forbidden) {
					t.Fatalf("%s still uses environment fallback %q", tt.path, forbidden)
				}
			}
		})
	}
}

func TestWindowsConfigCreationRejectsReparsePaths(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "cmd", "splitter", "config_windows.go"))
	for _, fragment := range []string{
		`func writeWindowsJSONConfigIfMissing(path string, cfg windowsJSONConfig) error`,
		`rejectWindowsReparsePath(dir)`,
		`windowsMkdirAll(dir, 0o755)`,
		`rejectWindowsReparsePath(path)`,
		`info, err := f.Stat()`,
		`config path must be a regular file`,
		`existing, _, err := openRegularConfigFile(configPath)`,
		`hardenWindowsConfigFileACL(configPath)`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("cmd/splitter/config_windows.go does not contain required config creation guard %q", fragment)
		}
	}
	for _, forbidden := range []string{
		`if err := os.MkdirAll(dir, 0o755); err != nil {`,
		`if _, err := os.Stat(configPath); err == nil {`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("cmd/splitter/config_windows.go still uses unsafe config creation fragment %q", forbidden)
		}
	}
}

func TestMSIHelperPurgeUsesCheckedRemoval(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "cmd", "gov-pass-msi-helper", "path_guard_windows.go"))
	for _, fragment := range []string{
		`removeCheckedWindowsPath(path, false)`,
		`removeCheckedWindowsPath(path, true)`,
		`windowsPathHasReparsePoint(path)`,
		`refusing to remove replaced non-directory`,
		`refusing to remove replaced directory as file`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("cmd/gov-pass-msi-helper/path_guard_windows.go does not contain required purge guard %q", fragment)
		}
	}
}

func TestWindowsServiceLogRotationRejectsReparsePoints(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "cmd", "splitter", "service_windows.go"))
	for _, fragment := range []string{
		`openServiceLogFile(path)`,
		`prepareWindowsServiceLogDir(dir, managedProgramDataPath)`,
		`var ensureSecureWindowsServiceLogDir = ensureSecureWindowsDir`,
		`ensureSecureWindowsServiceLogDir(dir)`,
		`rejectWindowsReparsePath(dir)`,
		`windowsMkdirAll(dir, 0o755)`,
		`windows.CreateFile(`,
		`windows.FILE_FLAG_OPEN_REPARSE_POINT`,
		`windows.FileAttributeTagInfo`,
		`lstatServiceLogFile(path)`,
		`rejectServiceLogReparsePoint(dst)`,
		`removeRotatedServiceLog(oldest)`,
		`windowsPathHasReparsePoint(path)`,
		`refusing service log reparse point`,
		`service log path is not a regular file`,
		`os.Lstat(path)`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("cmd/splitter/service_windows.go does not contain required log rotation guard %q", fragment)
		}
	}
	for _, forbidden := range []string{
		`info, err := os.Stat(path)`,
		`os.MkdirAll(dir, 0o755)`,
		`os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY`,
		`_ = os.Remove(oldest)`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("cmd/splitter/service_windows.go still uses unsafe log rotation fragment %q", forbidden)
		}
	}
}

func TestWindowsACLHardeningRejectsReparsePaths(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "cmd", "splitter", "acl_windows.go"))
	for _, fragment := range []string{
		`var windowsMkdirAll = os.MkdirAll`,
		`rejectWindowsReparsePath(dir)`,
		`windowsMkdirAll(dir, 0o755)`,
		`rejectWindowsReparsePath(path)`,
		`command name must be a bare filename`,
		`filepath.Base(name) != name`,
		`filepath.VolumeName(name) != ""`,
		`rejectWindowsReparsePath(path)`,
		`inspectManagedWindowsPath(path)`,
		`refusing reparse point path`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("cmd/splitter/acl_windows.go does not contain required ACL guard %q", fragment)
		}
	}
	if strings.Count(text, `rejectWindowsReparsePath(dir)`) < 3 {
		t.Fatal("cmd/splitter/acl_windows.go should check directory reparse state before mkdir, after mkdir, and before icacls")
	}
	for _, forbidden := range []string{
		`if err := os.MkdirAll(dir, 0o755); err != nil {`,
		`if _, err := os.Stat(path); err != nil {`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("cmd/splitter/acl_windows.go still uses unsafe ACL hardening fragment %q", forbidden)
		}
	}
}

func TestWinDivertExtractionRejectsUnsafeDestinations(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "internal", "driver", "windivert_download_windows.go"))
	text += readTextFile(t, filepath.Join(root, "internal", "driver", "path_reparse_windows.go"))
	for _, fragment := range []string{
		`rejectWinDivertExtractDestination(destPath)`,
		`var winDivertMkdirAll = os.MkdirAll`,
		`prepareWinDivertDirectory(destDir)`,
		`prepareWinDivertDirectory(dir)`,
		`rejectWinDivertPathReparseComponents(path)`,
		`winDivertGetFileAttributes`,
		`validateWinDivertDirectory(dir)`,
		`winDivertPathHasReparsePoint(path)`,
		`windows.FILE_ATTRIBUTE_REPARSE_POINT`,
		`errors.Is(err, os.ErrNotExist)`,
		`os.Lstat(path)`,
		`refusing to overwrite symlink WinDivert destination`,
		`refusing to overwrite reparse point WinDivert destination`,
		`refusing to overwrite non-regular WinDivert destination`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("internal/driver/windivert_download_windows.go does not contain required extraction guard %q", fragment)
		}
	}
	for _, forbidden := range []string{
		`rejectExistingWinDivertDirectorySymlink`,
		`os.MkdirAll(destDir, 0o755)`,
		`os.MkdirAll(dir, 0o755)`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("WinDivert extraction still uses unsafe directory creation fragment %q", forbidden)
		}
	}
	if strings.Contains(text, `_ = os.Remove(destPath)
	if err := os.Rename(tmpPath, destPath); err != nil {`) && !strings.Contains(text, `if err := rejectWinDivertExtractDestination(destPath); err != nil {
		return err
	}
	_ = os.Remove(destPath)`) {
		t.Fatal("WinDivert extraction removes destination without immediate safety guard")
	}
}

func TestWinDivertDLLConfigurationRejectsReparsePoints(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "internal", "adapter", "windivert_windows.go"))
	for _, fragment := range []string{
		`winDivertDLLPathHasReparsePoint(abs)`,
		`windows.GetFileAttributes(ptr)`,
		`windows.FILE_ATTRIBUTE_REPARSE_POINT`,
		`WinDivert.dll path must not be a reparse point`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("internal/adapter/windivert_windows.go does not contain required DLL reparse guard %q", fragment)
		}
	}
}

func TestWindowsDriverPowerShellScriptsPinSystem32SC(t *testing.T) {
	root := repoRoot(t)
	scripts := []string{
		filepath.Join("scripts", "install_windivert.ps1"),
		filepath.Join("scripts", "uninstall_windivert.ps1"),
	}

	for _, script := range scripts {
		t.Run(script, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, script))
			for _, fragment := range []string{
				`$ErrorActionPreference = "Stop"`,
				`function Test-ReparsePoint`,
				`function Assert-NoReparsePath`,
				`[Environment]::SystemDirectory`,
				`System32 command name must be a bare filename`,
				`Join-Path $sysDir $Name`,
				`Required System32 command must not be a reparse point`,
				`Assert-NoReparsePath $path "Required System32 command"`,
				`Assert-ServiceName $ServiceName`,
				`$ScExe = Get-System32Command "sc.exe"`,
				`& $ScExe`,
			} {
				if !strings.Contains(text, fragment) {
					t.Fatalf("%s does not contain required hardening fragment %q", script, fragment)
				}
			}
			for _, forbidden := range []string{
				`sc.exe query`,
				`sc.exe create`,
				`sc.exe qc`,
				`sc.exe config`,
				`sc.exe start`,
				`sc.exe stop`,
				`sc.exe delete`,
			} {
				if strings.Contains(text, forbidden) {
					t.Fatalf("%s still uses PATH-based sc.exe invocation %q", script, forbidden)
				}
			}
		})
	}
}

func TestWindowsInstallWinDivertValidatesDriverFileName(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "scripts", "install_windivert.ps1"))
	for _, fragment := range []string{
		`Assert-DriverFileName $SysName`,
		`SysName must be a bare .sys filename.`,
		`SysName must not use a reserved Windows device name.`,
		`TrimEnd([char[]]@(" ", "."))`,
		`$upperStem -match '^(COM[1-9]|LPT[1-9])$'`,
		`function Assert-SafeDriverDirectory`,
		`WinDivertDir must not be a reparse point`,
		`Assert-NoReparsePath $Path "WinDivertDir"`,
		`function Assert-SafeDriverFile`,
		`WinDivert driver sys must not be a reparse point`,
		`Assert-NoReparsePath $Path "WinDivert driver sys"`,
		`Resolve-Path -LiteralPath $sysPath`,
		`Test-Path -LiteralPath $normalized -PathType Leaf`,
		`Test-ReparsePoint $normalized`,
		`Assert-NoReparsePath $normalized "existing service bin path"`,
		"$outText = ($out -join \"`n\")",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("scripts/install_windivert.ps1 does not contain required hardening fragment %q", fragment)
		}
	}
}

func TestWindowsDriverFileNameRejectsReservedDeviceNames(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "internal", "driver", "validate_windows.go"))
	for _, fragment := range []string{
		`isReservedWindowsDeviceFileName(stem)`,
		`"CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$"`,
		`strings.HasPrefix(stem, "COM")`,
		`strings.HasPrefix(stem, "LPT")`,
		`driver sys name must not use reserved Windows device name`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("internal/driver/validate_windows.go does not contain required reserved device guard %q", fragment)
		}
	}
}

func TestWindowsPackageScriptUsesLiteralPaths(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "scripts", "package.ps1"))
	for _, fragment := range []string{
		`$ErrorActionPreference = "Stop"`,
		`function Test-ReparsePoint`,
		`Get-Item -LiteralPath $Path -Force`,
		`function Assert-NoReparsePath`,
		`function New-SafeDirectory`,
		`[IO.Directory]::CreateDirectory([IO.Path]::GetFullPath($Path))`,
		`must not contain a reparse point`,
		`function Assert-SafeSourceFile`,
		`Assert-NoReparsePath $Path "$Label source"`,
		`function Assert-SafeSourceDirectory`,
		`Assert-NoReparsePath $Path "$Label source directory"`,
		`function Copy-SafeFile`,
		`Assert-SafeSourceFile $Source $Label`,
		`Assert-ParentNoReparsePath $Destination`,
		`Assert-SafeSourceFile $ExePath "ExePath"`,
		`Assert-SafeSourceDirectory $WinDivertDir "WinDivertDir"`,
		`Resolve-Path -LiteralPath $ExePath`,
		`Assert-NoReparsePath $destExe "splitter exe destination"`,
		`Copy-Item -Force -LiteralPath`,
		`Get-ChildItem -LiteralPath $schemaSrc -File`,
		`Get-ChildItem -LiteralPath $examplesSrc -Filter "splitter.*.json" -File`,
		`Copy-SafeFile $_.FullName (Join-Path $schemaOut $_.Name) $_.Name`,
		`Copy-SafeFile $_.FullName (Join-Path $examplesOut $_.Name) $_.Name`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("scripts/package.ps1 does not contain required hardening fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{
		`Test-Path $ExePath`,
		`Test-Path $WinDivertDir`,
		`New-Item -ItemType Directory -Force -Path`,
		`Copy-Item -Force $ExePath`,
		`Copy-Item -Force (Join-Path $schemaSrc "*")`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("scripts/package.ps1 still uses wildcard-sensitive path operation %q", forbidden)
		}
	}
}

func TestWindowsOneTouchInstallerUsesLiteralPaths(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "scripts", "install_one_touch.ps1"))
	for _, fragment := range []string{
		`$ErrorActionPreference = "Stop"`,
		`function Test-ReparsePoint`,
		`function Assert-NoReparsePath`,
		`function New-SafeDirectory`,
		`function Assert-SafeOutputFilePath`,
		`function Resolve-GoCommand`,
		`GOV_PASS_GO_BIN`,
		`[Environment+SpecialFolder]::ProgramFiles`,
		`Resolve-Path -LiteralPath`,
		`Test-Path -LiteralPath $goPath -PathType Leaf`,
		`$goExe = Resolve-GoCommand`,
		`New-SafeDirectory $distDir "dist directory"`,
		`Assert-SafeOutputFilePath $splitterExe "splitter.exe"`,
		`& $goExe build -o $splitterExe`,
		`Test-Path -LiteralPath $packageScript -PathType Leaf`,
		`package.ps1 must not be a reparse point`,
		`Assert-NoReparsePath $packageScript "package.ps1"`,
		`Test-Path -LiteralPath $installDriverScript -PathType Leaf`,
		`install_windivert.ps1 must not be a reparse point`,
		`Assert-NoReparsePath $installDriverScript "install_windivert.ps1"`,
		`Push-Location -LiteralPath $repoRoot`,
		`& $packageScript -ExePath $splitterExe -WinDivertDir $windivertDir -OutDir $distDir`,
		`& $installDriverScript -WinDivertDir $distDir`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("scripts/install_one_touch.ps1 does not contain required hardening fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{
		`Resolve-Path (Join-Path`,
		`Push-Location $repoRoot`,
		`go build -o $splitterExe`,
		`Get-Command go`,
		`& (Join-Path $repoRoot "scripts\package.ps1")`,
		`& (Join-Path $repoRoot "scripts\install_windivert.ps1")`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("scripts/install_one_touch.ps1 still uses wildcard-sensitive fragment %q", forbidden)
		}
	}
}

func TestWindowsMSIE2EPinsSystem32AndKnownFolders(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "scripts", "windows", "ci_msi_e2e.ps1"))
	for _, fragment := range []string{
		`function Get-System32Command`,
		`$MsiExecPath = Get-System32Command "msiexec.exe"`,
		`$ScExePath = Get-System32Command "sc.exe"`,
		`Start-Process -FilePath $script:MsiExecPath`,
		`& $ScExePath control $svcName paramchange`,
		`function Test-ReparsePoint`,
		`function Assert-NoReparsePoint`,
		`function Assert-NoReparsePath`,
		`Assert-NoReparsePath -Path $path -Label "System32 command"`,
		`function Remove-SafeFileIfPresent`,
		`Remove-SafeFileIfPresent -Path $cfgPath -Label "config"`,
		`function Get-SafeTextFile`,
		`function Set-SafeTextFile`,
		`Set-SafeTextFile -Path $cfgPath`,
		`[Environment+SpecialFolder]::CommonApplicationData`,
		`[Environment+SpecialFolder]::ProgramFiles`,
		`Test-Path -LiteralPath`,
		`Get-Content -Raw -LiteralPath`,
		`Set-Content -Encoding ASCII -LiteralPath`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("scripts/windows/ci_msi_e2e.ps1 does not contain required hardening fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{
		`Start-Process -FilePath "msiexec.exe"`,
		`& sc.exe`,
		`$env:ProgramData`,
		`$env:ProgramFiles`,
		`Test-Path $Path`,
		`Get-Content -Raw -Path`,
		`Set-Content -Encoding ASCII -Path`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("scripts/windows/ci_msi_e2e.ps1 still uses unsafe fragment %q", forbidden)
		}
	}
}
