# Code Signing (Windows)

This project supports signing Windows release artifacts (EXE + MSI) in CI using
`osslsigncode` and a code signing certificate (PFX/PKCS12).

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
  - RFC3161 timestamp server URL.
  - Default: `http://timestamp.digicert.com`

## Local Signing (WSL/Linux)

If you have the PFX and password, you can sign local artifacts using:

```bash
sudo apt-get install -y --no-install-recommends osslsigncode

export WINDOWS_CODESIGN_PFX_B64="..."
export WINDOWS_CODESIGN_PFX_PASSWORD="..."
export WINDOWS_CODESIGN_TIMESTAMP_URL="http://timestamp.digicert.com"
export WINDOWS_CODESIGN_DESC="gov-pass"
export WINDOWS_CODESIGN_URL="https://example.com"

bash scripts/ci/sign_windows_artifacts.sh dist/release/gov-pass-*-windows-amd64.msi
```
