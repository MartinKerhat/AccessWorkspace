# Architecture

**Audience:** contributors and reviewers. **Covers:** what exists today — planned
work is not described here.

## Shape

One backend service, one frontend application, one PostgreSQL database. A
modular monolith, not microservices.

Two companion clients talk to the same backend rather than being separate
platforms:

- a local desktop **launcher** for SSH and RDP (Windows and Linux)
- a **browser extension** for portal credential fill (Chrome, Edge, Firefox)

## Components

### Backend (Go)

- authentication and identity mapping
- resource catalog API
- secret access policy and provider integration
- launch payload generation and one-time launch tickets
- audit logging
- expiry tracking and reminder delivery
- integration adapters for Azure/Entra and Azure Key Vault
- in-process background sync jobs

### Frontend (React + TypeScript)

- category-based workspace navigation, derived from rights: users see only
  categories they may use, `Activity` is visible to every signed-in user, and
  `Administration` appears only for admin-capable users
- category-specific list, search, and detail views
- reveal and copy actions, launcher handoff
- admin management workflows
- Key Vault browsing, app registration views, notification center

Categories: `Connections`, `Key Vault`, `App registrations`, `Passwords`, plus
the `Activity` and `Administration` workspace areas.

The web UI is the main operational console; the launcher and extension extend it
rather than replacing parts of it.

### PostgreSQL

Stores catalog metadata, access policy mapping, audit trail, imported external
metadata, expiry state and reminders, and ownership/lifecycle fields.

It deliberately does **not** become the permanent storage layer for every real
secret: values owned by an external system stay there and are fetched on demand.
Per-category storage and retrieval rules are in [object-model.md](object-model.md).

### Local launcher

- exposes a localhost bridge (`/status`, `/launch`) the web app checks before
  connect, reporting version, platform, and per-feature capabilities so the UI
  can warn about a missing prerequisite before a launch is attempted
- redeems one-time backend launch tickets, so secrets never travel inside
  browser URIs
- executes RDP with credential handoff and Remote Desktop Gateway support — on
  Windows through signed per-connection `.rdp` profiles and `mstsc`, on Linux
  through the system FreeRDP client with the password passed over stdin (no
  profiles or signing needed)
- executes SSH in a visible terminal, including launcher-managed password
  sessions (Windows console; auto-detected terminal emulator on Linux)
- self-installs per user, registers the `access-workspace://` handler, and
  arranges bridge autostart (registry on Windows, XDG desktop entries on Linux)

The launch payload stays **semantic** — host, port, user, domain, gateway,
options. Platform mechanics (`.rdp` files, `cmdkey`, `rdpsign`, FreeRDP
arguments) are derived by the platform adapter, never sent by the backend.

### Browser extension

- connects through a one-time exchange token and holds its own session
- requests approved portal credentials from backend flows
- fills credentials on allowed websites and can save new personal logins back
  into the workspace
- logs sensitive fill actions to the audit trail

## Security architecture

Summarized here; the full picture is in [security.md](security.md).

- **Secrets at rest:** envelope encryption per secret, with the key-encryption
  key either derived locally or held inside Azure Key Vault via workload
  identity. Sensitive admin settings are encrypted; session tokens are stored
  only as hashes.
- **Personal vault:** a per-user keypair whose public half encrypts (so saving
  never prompts) and whose private half exists only wrapped by the user's unlock
  methods — login password, passphrase, or per-device passkey. Administrators
  and database access cannot read personal secrets, and losing every unlock
  method loses the vault by design.
- **Sessions and perimeter:** httpOnly cookie sessions with CSRF origin checks,
  account lockout and per-IP throttling on auth endpoints, CSP/HSTS, and audit
  coverage of authentication and vault events.

## Principles

- Keep one deployable backend until complexity truly demands otherwise.
- Use adapters for external systems; let the system of record stay the system of
  record.
- Separate metadata from secret retrieval.
- Treat reveal, copy, launch, and fill as distinct action types.
- Keep the launch payload semantic and platform mechanics in the adapter.
- Use small in-process background jobs where they fit.

## Data flow

### Authentication and authorization

Microsoft Entra sign-in with local-account fallback, invites, and self-service
password change; a development auth mode for local work. Sessions are httpOnly
cookies with CSRF origin checks, protected by account lockout and IP throttling.
Authorization resolves category capabilities and roles from local groups and
mapped external groups; admins can inspect any user's effective access.

### Secret access

App-managed values are stored under envelope encryption in three classes:
shared (readable by authorized users under policy), personal (sealed to the
owner's vault), and app-scope (integration credentials the backend needs without
a session). Personal/shared switching re-wraps keys server-side without exposing
plaintext and is owner-only. Key Vault-backed values are fetched on demand and
never persisted locally. Reveal, copy, fill, and launch are distinct audited
actions.

### Launching

The backend returns a structured launch payload and a one-time ticket. The web
app verifies launcher presence, version, and capabilities through the localhost
bridge, then hands off. The launcher executes RDP and SSH natively per platform.
Web portal launches open in the browser, with extension-assisted fill where
allowed.

### External integrations

- Key Vault adapter: discovery, batch import, manual and automatic sync,
  archived/restore views
- App registration adapter: discovery, import, sync, owner snapshots, credential
  expiry metadata
- Expiry reminders for app registration credentials and Key Vault secrets, in
  the notification center and over SMTP, with a delivery log
- Admin-managed Entra and Key Vault runtime configuration; Azure access can run
  through a dedicated reader identity (workload identity) separate from the
  OIDC login app

## Deployment notes

Local Docker Compose is the primary developer workflow; backend and frontend
stay easy to containerize. AKS readiness means clear configuration, health
endpoints, and clean service boundaries — not premature service splitting. See
[deployment.md](deployment.md).
