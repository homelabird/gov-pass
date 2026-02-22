# Code Signing (Windows)

This project supports signing Windows release artifacts (EXE + MSI) in CI using
`osslsigncode` and a code signing certificate (PFX/PKCS12).

## SmartScreen Notes

- SmartScreen warnings/blocks are not based on signing alone; SmartScreen is reputation-based.
- To meaningfully reduce SmartScreen warnings, you need an Authenticode signature from a trusted public CA
  (OV or EV code signing). Self-signed certificates do not help for end users.
- Even with a valid signature, new publishers may still see SmartScreen warnings until reputation is established.
  EV certificates often reduce friction, but they usually require hardware-backed or cloud signing.

## What Gets Signed

On tag builds, GitLab CI signs:
- `splitter.exe`
- `gov-pass-tray.exe`
- `gov-pass-msi-helper.exe`
- `gov-pass-<tag>-windows-amd64.msi`

The signed EXEs are included in the Windows zip, and the MSI is signed after it
is built.

## CI Variables

Configure these GitLab CI variables (recommended: masked + protected):

- `WINDOWS_CODESIGN_PFX_B64`
  - Base64-encoded PFX/PKCS12 file content.
- `WINDOWS_CODESIGN_PFX_PASSWORD`
  - Password for the PFX.

Tag release builds require these variables; otherwise the `build_release` job
fails before producing artifacts.

Optional:
- `WINDOWS_CODESIGN_TIMESTAMP_URL`
  - RFC3161 timestamp server URL (single).
- `WINDOWS_CODESIGN_TIMESTAMP_URLS`
  - RFC3161 timestamp server URLs (comma or space separated; tried in order).
  - Default: `http://timestamp.digicert.com,http://timestamp.sectigo.com`

## Verify Signatures

PowerShell:

```powershell
Get-AuthenticodeSignature .\dist\release\gov-pass-*-windows-amd64.msi |
  Format-List Status,StatusMessage,SignerCertificate,TimeStamperCertificate
```

For CI, `scripts/windows/ci_msi_e2e.ps1` asserts that the MSI and installed EXEs are signed.

## EV / Hardware-Backed Keys

Many EV code signing certificates cannot be exported as a PFX (USB token or cloud signing).
If you use EV:
- this repo's current CI signing flow (PFX base64 variables + `osslsigncode`) may not apply as-is
- you typically sign on a Windows runner with the vendor tooling (`signtool.exe`) or integrate the CA's cloud signing

## Local Signing (WSL/Linux)

If you have the PFX and password, you can sign local artifacts using:

```bash
sudo apt-get install -y --no-install-recommends osslsigncode

export WINDOWS_CODESIGN_PFX_B64="..."
export WINDOWS_CODESIGN_PFX_PASSWORD="..."
export WINDOWS_CODESIGN_TIMESTAMP_URLS="http://timestamp.digicert.com,http://timestamp.sectigo.com"
export WINDOWS_CODESIGN_DESC="gov-pass"
export WINDOWS_CODESIGN_URL="https://example.com"

bash scripts/ci/sign_windows_artifacts.sh dist/release/gov-pass-*-windows-amd64.msi
```
