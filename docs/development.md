# Development

**Audience:** anyone building or testing the app locally. **Covers:** what exists today.

## Repository layout

- `backend` — Go API, PostgreSQL migrations, seed command, tests
- `frontend` — React + TypeScript UI
- `launcher` — Go desktop launcher (Windows, Linux, macOS)
- `browser-extension` — Chrome, Firefox and Safari extension sources, plus the Xcode wrapper project Safari needs
- `deploy` — Dockerfiles and deployment-oriented assets
- `docs` — documentation of what the app does today
- `scripts` — build helpers

## Run with Docker

1. From the repo root, copy the environment template and fill in values:

   ```powershell
   Copy-Item .env.example .env
   ```

   At minimum set `RESOURCE_SECRET_KEY` and `POSTGRES_PASSWORD`. Compose reads `.env`
   and fails fast with a clear message if a required secret is missing. `.env` is
   gitignored and must never be committed.

2. Run `docker compose up --build`.
3. Open <http://localhost:4173>.
4. The backend API is available at <http://localhost:8080>.

The backend runs migrations automatically. For local convenience the compose file
defaults `SEED_ON_START=true`, so demo users and sample data are available out of the
box. Set `SEED_ON_START=false` (or `RESET_DB_ON_START=true` for a clean reset) in
`.env` to change that.

## Run without Docker

### Backend

1. Start PostgreSQL locally and create a database named `access_workspace`.
2. From `backend`, run:

```powershell
$env:DATABASE_URL="postgres://postgres:postgres@localhost:5432/access_workspace?sslmode=disable"
$env:RESOURCE_SECRET_KEY="a-unique-local-dev-key"
$env:AUTO_MIGRATE="true"
$env:RESET_DB_ON_START="true"
$env:SEED_ON_START="true"
go run ./cmd/server
```

`RESOURCE_SECRET_KEY` is required — the server refuses to start without it. Leave
`RESET_DB_ON_START` unset or set it to `false` to preserve local database state.
See [Configuration](configuration.md) for the full variable list.

To seed manually instead of on startup:

```powershell
go run ./cmd/seed
```

### Frontend

From `frontend`:

```powershell
npm install
npm run dev
```

Then open <http://localhost:5173>.

### Browser extension

The extension is plain JavaScript with no build step; load the source folder
directly:

- Edge / Chrome: `edge://extensions` (or `chrome://extensions`) → enable
  *Developer mode* → *Load unpacked* → select `browser-extension/chrome`.
  After changing a file, press *Reload* on the extension card.
- Firefox: `about:debugging#/runtime/this-firefox` → *Load Temporary Add-on* →
  select `browser-extension/firefox/manifest.json`.
- Safari (macOS with Xcode): Safari runs extensions only from inside a Mac
  app, so build the wrapper first: `scripts/build-safari-extension.sh`
  (unsigned developer build) or open
  `browser-extension/safari-app/Access Workspace Browser Fill.xcodeproj` in
  Xcode and run it. Then enable *Develop → Allow Unsigned Extensions* in
  Safari, turn the extension on under *Settings → Extensions*, and allow it
  on the workspace site when Safari asks. The Xcode project references
  `browser-extension/safari` directly, so edits there are picked up on the
  next build. If you add files to the extension or change the manifest's
  structure, regenerate the wrapper with
  `xcrun safari-web-extension-converter browser-extension/safari --rebuild-project browser-extension/safari-app/Access\ Workspace\ Browser\ Fill.xcodeproj`,
  then re-check the hand-made parts of `project.pbxproj`: the bundle ids
  (`…accessworkspace.browserfill` / `….browserfill.Extension`), macOS 13.0
  deployment target, and `CODE_SIGN_ENTITLEMENTS` pointing at the two
  `.entitlements` files. Those files keep the App Sandbox entitlement, which
  Safari requires: without it the extension silently never appears in
  Safari's Extensions list.

Then open the running workspace, choose *Browser extensions* in the account
menu and connect; the extension stores the workspace URL it was connected
from, so a local stack on `http://localhost:8080` works without any extra
configuration.

## Tests

Backend unit tests cover auth and capability behavior, permission filtering,
audit logging, archive/restore, Key Vault sync and auto-import, app registration
discovery/import/sync, expiry status and reminder policy resolution, and
envelope encryption (round-trip, legacy-format reads, tamper detection, KEK
provider switching, personal-envelope sealing).

Run them from `backend`:

```powershell
go test ./...
```

Some end-to-end tests exercise real database behavior (migrations, startup
re-encryption, invite/reset, personal↔shared switching, passphrase unlock). They
are skipped unless `VERIFY_DATABASE_URL` points at a throwaway database:

```powershell
$env:VERIFY_DATABASE_URL="postgres://postgres:postgres@localhost:5432/verify_db?sslmode=disable"
go test ./...
```

Frontend type checking and build:

```powershell
npx tsc --noEmit
npm run build
```

### macOS launcher

The macOS build needs a Mac with Xcode's command line tools, Go and CMake
(`brew install cmake`). `launcher/scripts/build_macos.sh` produces the
`.dmg`; the first run also compiles the bundled FreeRDP client and OpenSSL
for both architectures (`launcher/scripts/build_freerdp_macos.sh`, ten
minutes or so, cached afterwards under `artifacts/launcher/mac-deps`).
Without a Developer ID the result is ad-hoc signed and runs only on the Mac
that built it: drag it into `/Applications` and open it once, as a user would.
`MACOS_SKIP_FREERDP=1` builds an SSH-only launcher in seconds when RDP is not
what you are working on.

### What cannot be tested headlessly

- The WebAuthn passkey ceremony (Windows Hello / Touch ID). Its server-side
  crypto is unit-tested; the browser flow is verified on an HTTPS deployment.
- Launcher RDP/SSH execution. Argument and profile construction is unit-tested,
  but launching a real session needs a desktop with the native client installed.
