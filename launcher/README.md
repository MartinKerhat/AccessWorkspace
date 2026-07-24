# Access Workspace Launcher

This folder contains the local desktop launcher for the Connections module
(Windows and Linux; macOS is planned).

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

Current limits:

- macOS is not supported yet
- SSH still assumes password-based auth for the managed flow and falls back to the native client for key-driven setups
- RDP signing (Windows) currently assumes a workspace-managed test or uploaded certificate package instead of a production enterprise PKI workflow

## Build

From this folder, `.\build.ps1` builds and packages both platform artifacts
into `../artifacts/launcher/`. For a quick local Windows build:

```powershell
go build -o dist\access-workspace-launcher.exe .\cmd\access-workspace-launcher
```

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

- `access-workspace-launcher-windows-amd64-v0.5.6.exe`

Run the `.exe` once. On Windows, that first run now:

- copies the launcher into `%LOCALAPPDATA%\AccessWorkspaceLauncher\access-workspace-launcher.exe`
- registers the `access-workspace://` protocol to that installed copy
- registers `HKCU\Software\Microsoft\Windows\CurrentVersion\Run\AccessWorkspaceLauncher`
- starts the background bridge immediately with `--background`
- shows a confirmation dialog

After that, `Connect` from the web app first checks the local launcher version through the background bridge and only then hands the connection into the installed helper.
