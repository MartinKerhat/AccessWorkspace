# Access Workspace Launcher

This folder contains the local desktop launcher for the Connections module
(Windows, Linux and macOS).

Shared behavior on every platform:

- exposes a local launcher bridge on `http://127.0.0.1:47654`
- reports launcher version, platform, and per-feature capabilities to the web app through `/status`
- accepts local web-app launch calls through `/launch`
- redeems one-time backend launch tickets so secrets do not travel inside the browser URI
- opens web targets through the platform browser
- self-installs when the executable is run without launch arguments, registers the `access-workspace://` handler, and arranges autostart of the bridge

Windows specifics:

- starts native RDP through `mstsc.exe`, with temporary credential handoff via `cmdkey.exe`
- maintains stable per-connection `.rdp` profiles under `%LOCALAPPDATA%\AccessWorkspaceLauncher\profiles\rdp`, signed with `rdpsign.exe`, and installs the workspace-managed RDP signing certificate chain when needed
- starts launcher-managed SSH password sessions in a visible console; falls back to the native SSH client when no stored password is present

Linux specifics:

- starts RDP through the system FreeRDP client (`xfreerdp`/`xfreerdp3`), mapping connection fields — target, credentials, domain, Remote Desktop Gateway, admin session — to arguments; the password is handed over on stdin (`/from-stdin`), never on the command line. No profile files and no profile signing are involved.
- FreeRDP is a preflight-checked system package (`sudo apt install freerdp3-x11` / `sudo dnf install freerdp`); when missing, the web app shows an install hint via the capability report
- starts launcher-managed SSH sessions inside the user's terminal emulator (auto-detected: `xdg-terminal-exec`, gnome-terminal, konsole, and friends); key-driven setups fall through to the native `ssh` client in the same terminal
- installs per-user with XDG conventions: binary in `~/.local/bin`, URI-handler `.desktop` entry, autostart entry, state under `~/.local/share/access-workspace-launcher`
- install from the tarball: `tar -xzf access-workspace-launcher-linux-amd64-v*.tar.gz && ./access-workspace-launcher`

macOS specifics:

- ships as one signed, notarized `.dmg` with everything inside: the universal (Apple silicon + Intel) launcher app and, under `Contents/Helpers`, FreeRDP's native Cocoa client (`MacFreeRDP`, built from source with OpenSSL linked statically by `scripts/build_freerdp_macos.sh`). Mac users are not asked to find and install an RDP client — the opposite of the Linux model.
- starts RDP through that bundled client with the same argument mapping as Linux (target, credentials, domain, gateway, admin session, window title; password on stdin via `/from-stdin`), in a window sized to the display with smart sizing; the green button goes fullscreen
- starts launcher-managed SSH sessions in Terminal.app (part of macOS, next to the system OpenSSH client), through a self-deleting `.command` script so no Automation permission prompt appears
- installs the platform way: drag the app from the `.dmg` to Applications and open it once. That first open registers a per-user LaunchAgent (`~/Library/LaunchAgents/io.github.martinkerhat.accessworkspace.launcher.plist`) that keeps the localhost bridge running and restarts it at login; the `access-workspace://` scheme comes from the bundle's `Info.plist`, registered by LaunchServices. Upgrading is the same gesture: replace the app, open it once.
- state lives under `~/Library/Application Support/access-workspace-launcher` (logs, payloads)
- no Dock icon or menu bar (`LSUIElement`): the launcher is a background bridge; the RDP session window is a normal app window

Current limits:

- SSH still assumes password-based auth for the managed flow and falls back to the native client for key-driven setups
- RDP signing (Windows) currently assumes a workspace-managed test or uploaded certificate package instead of a production enterprise PKI workflow

## Build

From this folder, `.\build.ps1` builds and packages the Windows and Linux
artifacts into `../artifacts/launcher/`; on a Mac, `scripts/build_macos.sh`
builds the macOS `.dmg` (first run also compiles the bundled FreeRDP client,
see `scripts/build_freerdp_macos.sh`; `MACOS_SKIP_FREERDP=1` for a quick
SSH-only build). For a quick local Windows build:

```powershell
go build -o dist\access-workspace-launcher.exe .\cmd\access-workspace-launcher
```

### Install from source

With a Go toolchain installed, the launcher can be built and installed straight
from the repository without cloning it:

```sh
go install github.com/MartinKerhat/AccessWorkspace/launcher/cmd/access-workspace-launcher@latest
```

The binary lands in `$(go env GOPATH)/bin` (`%USERPROFILE%\go\bin` on Windows).
Run it once to register the protocol handler, exactly as with a downloaded
release. Release tags (`launcher-v<version>`) are not Go module version tags, so
`@latest` resolves to the newest commit on `main`; pin a specific release
with `@launcher-v<version>` when reproducibility matters. Prebuilt, versioned
artifacts remain available from the GitHub Releases page and the in-app
download dialog.

## Test a payload

```powershell
.\dist\access-workspace-launcher.exe --uri "access-workspace://launch?payload=..."
```

or

```powershell
.\dist\access-workspace-launcher.exe --payload "..."
```

## Windows install

The frontend download folder includes the current versioned launcher artifact:

- `access-workspace-launcher-windows-amd64-v<version>.exe`

Run the `.exe` once. On Windows, that first run now:

- copies the launcher into `%LOCALAPPDATA%\AccessWorkspaceLauncher\access-workspace-launcher.exe`
- registers the `access-workspace://` protocol to that installed copy
- registers `HKCU\Software\Microsoft\Windows\CurrentVersion\Run\AccessWorkspaceLauncher`
- starts the background bridge immediately with `--background`
- shows a confirmation dialog

After that, `Connect` from the web app first checks the local launcher version through the background bridge and only then hands the connection into the installed helper.

## macOS install

- `access-workspace-launcher-macos-universal-v<version>.dmg`

Open the disk image, drag **Access Workspace Launcher** to Applications and open it once (Gatekeeper accepts it: the build is signed with a Developer ID and notarized). That first open:

- registers the per-user LaunchAgent that runs the background bridge (`--background`) now and at every login
- shows a confirmation dialog

The `access-workspace://` handler needs no registration step — LaunchServices picks it up from the app bundle. Opening the app while it sits in the `.dmg` or outside Applications is refused with a hint, because a LaunchAgent pointing there would break as soon as the image is ejected.
