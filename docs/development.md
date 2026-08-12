# Development

**Audience:** anyone building or testing the app locally. **Covers:** what exists today.

## Repository layout

- `backend` — Go API, PostgreSQL migrations, seed command, tests
- `frontend` — React + TypeScript UI
- `launcher` — Go desktop launcher (Windows, Linux; macOS pending)
- `browser-extension` — Chrome and Firefox extension sources
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

### What cannot be tested headlessly

- The WebAuthn passkey ceremony (Windows Hello / Touch ID). Its server-side
  crypto is unit-tested; the browser flow is verified on an HTTPS deployment.
- Launcher RDP/SSH execution. Argument and profile construction is unit-tested,
  but launching a real session needs a desktop with the native client installed.
