# Code Signing (Windows + Sigstore)

This project publishes two signature layers for release artifacts:

- Authenticode signatures for Windows binaries (`.exe`, `.msi`).
- Sigstore keyless signature bundle for the release checksum manifest (`SHA256SUMS`).

Authenticode and Sigstore solve different problems and are used together.

## Why Both

- Authenticode is what Windows checks for executable trust and installer UX.
- Sigstore provides supply-chain integrity for published release files without
  managing a long-lived signing key in CI.

Sigstore does not replace Authenticode for MSI install trust on Windows.

## SmartScreen Notes

- SmartScreen warnings/blocks are not based on signing alone; SmartScreen is reputation-based.
- To meaningfully reduce SmartScreen warnings, you need an Authenticode signature from a trusted public CA
  (OV or EV code signing). Self-signed certificates do not help for end users.
- Even with a valid signature, new publishers may still see SmartScreen warnings until reputation is established.
  EV certificates often reduce friction, but they usually require hardware-backed or cloud signing.

## What Gets Signed

On tag builds, GitLab CI signs with Authenticode:

- `splitter.exe`
- `gov-pass-tray.exe`
- `gov-pass-msi-helper.exe`
- `gov-pass-<tag>-windows-amd64.msi`

GitLab CI also produces:

- `SHA256SUMS`
- `SHA256SUMS.sigstore.json` (Sigstore bundle from `cosign sign-blob`)

The signed EXEs are included in the Windows zip, the MSI is signed after it is
built, and the final checksum manifest is signed with Sigstore keyless.

## CI Requirements

`build_release` is configured to request a GitLab OIDC token for Sigstore:

- `id_tokens` with `SIGSTORE_ID_TOKEN` and `aud: sigstore`
- `cosign sign-blob SHA256SUMS --bundle SHA256SUMS.sigstore.json`

No long-lived private key is stored for Sigstore keyless signing.

## CI Variables (Authenticode)

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

1. Verify Authenticode (MSI/EXE):

```powershell
Get-AuthenticodeSignature .\dist\release\gov-pass-*-windows-amd64.msi |
  Format-List Status,StatusMessage,SignerCertificate,TimeStamperCertificate
```

For CI, `scripts/windows/ci_msi_e2e.ps1` asserts that the MSI and installed EXEs are signed.

2. Verify Sigstore bundle (`SHA256SUMS`):

```bash
cosign verify-blob dist/release/SHA256SUMS \
  --bundle dist/release/SHA256SUMS.sigstore.json \
  --certificate-identity "https://gitlab.com/<group>/<project>//.gitlab-ci.yml@refs/tags/vX.Y.Z" \
  --certificate-oidc-issuer "https://gitlab.com"
```

Then verify artifact hashes from `SHA256SUMS`:

```bash
cd dist/release
sha256sum -c SHA256SUMS --ignore-missing
```

If using self-managed GitLab, replace issuer/identity with your GitLab host.

## EV / Hardware-Backed Keys

Many EV code signing certificates cannot be exported as a PFX (USB token or cloud signing).
If you use EV:

- this repo's current Authenticode CI flow (PFX base64 variables + `osslsigncode`) may not apply as-is
- you typically sign on a Windows runner with the vendor tooling (`signtool.exe`) or integrate the CA's cloud signing

## Local Signing (WSL/Linux, Authenticode)

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
