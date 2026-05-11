# Security Warnings: Unsigned Binaries

OpenBISS is currently distributed as unsigned binaries. This means the executable has not been cryptographically signed with a developer certificate recognized by macOS, Windows, or other platforms. As a result, your operating system may display a security warning or block the application from running on first launch.

This document explains what those warnings mean, how to proceed safely, and what OpenBISS plans to do about signing in the future.

---

## macOS (Gatekeeper)

macOS Gatekeeper checks whether downloaded applications are signed and notarized by Apple. Because OpenBISS binaries are not yet notarized, Gatekeeper will block the app on first run with a message like:

> "OpenBISS cannot be opened because it is from an unidentified developer."

You have two options to bypass this for a specific binary.

### Option 1: Remove the quarantine attribute (terminal)

When macOS downloads a file, it attaches a quarantine extended attribute. Removing it tells Gatekeeper to skip the check for that file.

```bash
xattr -d com.apple.quarantine /path/to/openbiss
```

Replace `/path/to/openbiss` with the actual path to the downloaded binary. After running this command, you can open the app normally.

### Option 2: Right-click to open (GUI)

1. In Finder, locate the OpenBISS binary or `.app` bundle.
2. Right-click (or Control-click) the file.
3. Select **Open** from the context menu.
4. A dialog appears asking if you're sure. Click **Open**.

macOS remembers this choice for the file, so subsequent launches work without the warning.

> **Do not** use `spctl --master-disable` to globally disable Gatekeeper. That removes protection for all applications, not just OpenBISS.

---

## Windows (SmartScreen)

Windows SmartScreen checks whether downloaded executables have a valid Authenticode signature and sufficient reputation. Unsigned binaries trigger a warning:

> "Windows protected your PC. Microsoft Defender SmartScreen prevented an unrecognized app from starting."

You have two options.

### Option 1: Unblock via file properties

1. Right-click the downloaded `.exe` file.
2. Select **Properties**.
3. On the **General** tab, look for a message at the bottom: "This file came from another computer and might be blocked..."
4. Check the **Unblock** checkbox.
5. Click **OK** or **Apply**.

The file is now unblocked and will run without the SmartScreen prompt.

### Option 2: "More info" then "Run anyway"

If you've already tried to run the file and see the SmartScreen dialog:

1. Click **More info** in the SmartScreen dialog.
2. The dialog expands to show the app name and publisher.
3. Click **Run anyway**.

> **Do not** disable SmartScreen globally in Windows Security settings. That removes protection for all downloaded software.

---

## Linux

Standard Linux desktop environments don't enforce code signing for downloaded executables the way macOS and Windows do. There's no equivalent of Gatekeeper or SmartScreen built into the kernel or common desktop environments.

Instead, verify the integrity of the downloaded binary using the provided SHA-256 checksum.

### Verifying the checksum

Each OpenBISS release includes a `checksums.txt` file alongside the binaries. To verify:

```bash
# Download the binary and checksums file, then:
sha256sum -c checksums.txt
```

Or verify a single file manually:

```bash
sha256sum openbiss-linux-amd64
# Compare the output against the expected hash in checksums.txt
```

If the hashes match, the binary arrived intact and hasn't been tampered with in transit. If they don't match, don't run the binary.

---

## Future Plans

OpenBISS intends to implement proper code signing for official releases:

- **macOS**: Sign and notarize binaries using an Apple Developer ID certificate. Notarized apps pass Gatekeeper automatically without any user intervention.
- **Windows**: Sign binaries with a Windows Authenticode certificate (EV or OV). Signed binaries with sufficient reputation skip the SmartScreen warning entirely.

No specific timeline is committed. Until signing is in place, the bypass methods above are the supported approach for running OpenBISS.

---

## Why Code Signing Matters

Code signing ties an executable to a specific developer identity. When you run a signed application, your OS verifies that the binary was produced by the stated developer and hasn't been modified since signing. If the signature is invalid or missing, the OS warns you.

This matters because unsigned binaries could theoretically be replaced by malicious software without you knowing. A signed binary gives you a chain of accountability: if something goes wrong, there's a developer identity attached to it.

Signing doesn't guarantee software is safe or bug-free. It only proves origin and integrity. A signed application can still contain vulnerabilities or behave maliciously. Signing is one layer of trust, not a complete security guarantee.

For open-source projects like OpenBISS, you can also verify trust by building from source. The repository is public, the build process is documented, and the resulting binary is deterministic given the same toolchain.

---

## OpenBISS Security Stance

OpenBISS is unsigned at the application-signing level. This is a distribution limitation, not a reflection of how OpenBISS handles document security.

When you use the `/sign` endpoint to sign a document, OpenBISS validates the signing certificate against your operating system's trust store. It does not maintain a custom or proprietary certificate store. This means:

- Certificates trusted by your OS are trusted by OpenBISS for signing operations.
- Certificates not in your OS trust store are rejected.
- OpenBISS does not bypass your system's certificate validation.

The unsigned binary warning is about whether your OS trusts the OpenBISS executable itself. The trust store validation in `/sign` is about whether OpenBISS trusts the certificates used to sign documents. These are separate concerns.

In short: OpenBISS may not yet have Apple's or Microsoft's blessing as a signed app, but it still enforces proper certificate validation when doing its actual job.
