# Access Workspace Launcher

This folder contains the first local launcher preview for the Connections module.

Current preview scope:

- exposes a local launcher bridge on `http://127.0.0.1:47654`
- reports launcher version and readiness to the web app through `/status`
- accepts local web-app launch calls through `/launch`
- redeems one-time backend launch tickets so secrets do not travel inside the browser URI
- starts launcher-managed SSH sessions on Windows when a stored password is available
- falls back to the native SSH client when no stored password is present
- starts native RDP on Windows through `mstsc.exe` command-line launch
- uses `cmdkey.exe` on Windows for temporary RDP credential handoff before opening `mstsc.exe`
- maintains stable per-connection `.rdp` profiles under `%LOCALAPPDATA%\AccessWorkspaceLauncher\profiles\rdp`
- installs the workspace-managed RDP signing certificate chain into the current Windows user stores when needed
- signs managed `.rdp` profiles with `rdpsign.exe` before launching `mstsc.exe`
- opens SSH on Windows in a visible `cmd.exe` session instead of a detached background process
- opens web targets through the platform browser
- self-installs on Windows when the executable is run without launch arguments
- registers an autorun entry on Windows so the local bridge is available after sign-in

Current limits:

- RDP preview is Windows-only
- SSH still assumes password-based auth for the managed Windows flow and falls back to the native client for key-driven setups
- RDP signing currently assumes a workspace-managed test or uploaded certificate package instead of a production enterprise PKI workflow

## Build

From this folder:

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
