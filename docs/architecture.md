# Architecture

## High-level approach

Keep deployment simple while designing clear internal boundaries for future growth.

Current deployment target:

- one backend service
- one frontend application
- one PostgreSQL database

Shipped companion clients:

- local desktop launcher for SSH and RDP (Windows and Linux; localhost bridge + one-time launch tickets)
- browser extension for portal credential fill

Both integrate with the same backend rather than becoming separate platforms.

## System components

### Backend

Primary responsibilities:

- authentication and identity mapping
- resource catalog API
- secret access policy and provider integration
- launch payload generation
- audit logging
- expiry tracking
- integration entry points for Azure/Entra, Key Vault, and future systems
- lightweight in-process background sync jobs

Recommended internal modules:

- `auth`
- `catalog`
- `secrets`
- `launch`
- `audit`
- `integrations`
- `jobs`

This is a modular monolith, not microservices.

### Frontend

Primary responsibilities:

- category-based workspace navigation
- category-specific list and search views
- resource detail views
- secret reveal/copy actions
- launch initiation
- admin management workflows
- audit and recent activity views
- Key Vault browsing UX
- expiry dashboards

The web UI should remain the main operational console even after helper and extension support are added.

### Frontend navigation model

The main workspace should be organized by object category, not one default mixed catalog.

Initial categories:

- `Connections`
- `Key Vault`
- `App registrations`
- `Passwords`
- `Activity`
- `Admin`

Navigation should be rights-aware:

- users should only see categories they are allowed to access
- `Activity` should be visible for signed-in users
- `Admin` should appear only for admin-capable users

### PostgreSQL

Primary responsibilities:

- catalog metadata
- access policy mapping
- audit trail
- imported external metadata
- expiry state and reminders
- ownership and lifecycle fields

PostgreSQL should store normalized operational metadata, category mapping, and policy state, not become the permanent storage layer for every real secret.

For category-specific storage and retrieval rules, see [object-model-spec.md](object-model-spec.md).

### Local launcher/helper

Current responsibilities (shipped: Windows and Linux):

- expose a localhost bridge (`/status`, `/launch`) the web app checks before connect, reporting version, platform, and per-feature capabilities
- redeem one-time backend launch tickets so secrets never travel inside browser URIs
- execute RDP launches with credential handoff and Remote Desktop Gateway support — on Windows through signed per-connection `.rdp` profiles and `mstsc`, on Linux through the system FreeRDP client with arguments (password over stdin, no profiles or signing needed)
- execute SSH launches in a visible terminal, including launcher-managed password sessions (Windows console; auto-detected terminal emulator on Linux)
- self-install per user, register the `access-workspace://` handler, and arrange bridge autostart (registry on Windows, XDG desktop entries on Linux)

The launch payload stays semantic (host, port, user, options) so each platform picks its native client mechanics; a macOS launcher is planned follow-through.

### Browser extension

Current responsibilities (shipped):

- connect to the workspace through a one-time exchange token and its own session
- request approved portal credentials from backend flows
- fill credentials on allowed websites, including saving new personal logins back to the workspace
- log sensitive fill actions to the audit trail

It remains separate from the main web UI.

## Security architecture

Delivered layers (2026-07):

### Secrets at rest

- every stored secret uses envelope encryption: a fresh per-secret data key encrypts the value, and the data key itself is wrapped by a key-encryption key
- the KEK provider is deployment configuration: a local key for development, or Azure Key Vault wrap/unwrap through workload identity for production
- sensitive admin settings are encrypted at rest; session tokens are stored only as hashes

### Personal vault

- every user has a personal vault keypair: the public half encrypts, so saving a personal secret works from any session without any prompt (including extension saves)
- the private half is never stored bare — it exists only wrapped by the user's unlock methods, and a database copy alone cannot open it
- unlock methods are peers of one another: the local login password (unlocks automatically at sign-in), a passphrase, and passkeys (Windows Hello / Touch ID via WebAuthn PRF, one per device)
- users manage their own methods from the vault settings UI: add a passphrase or per-device passkey, rename passkeys, and remove methods (the last remaining method and the login-password wrap are protected)
- consequence by design: administrators and database access cannot read personal secrets, and a user who loses every unlock method loses the vault — there are no recovery codes

### Sessions and perimeter

- web sessions ride an httpOnly cookie (no token in localStorage or redirect URLs) with a CSRF origin check; the extension keeps a separate bearer-token session
- login and vault-unlock endpoints have account lockout and per-IP rate limiting
- the frontend ships CSP, HSTS, and related security headers; the API sets equivalent headers on its responses
- authentication, vault, and unlock-method changes are audited alongside resource events

## Architectural principles

- Keep one deployable backend until complexity truly demands otherwise.
- Use adapters for external systems such as Azure/Entra and Key Vault.
- Separate metadata from secret retrieval.
- Treat reveal, copy, launch, and fill as distinct action types.
- Use small in-process background jobs when they fit, while keeping the option to split them later if operational pressure demands it.

## Data flow direction

### Authentication and authorization

Current state:

- development auth mode for local work
- Microsoft Entra sign-in with local-account fallback, invites, and self-service password change
- httpOnly-cookie sessions with CSRF origin checks; account lockout and IP throttling on auth endpoints
- backend authorization based on resolved category capabilities and roles
- admin user administration: effective-access inspection, local groups, blocking, resets

Next target:

- broader Entra group resolution hardening
- optional second factor for local-account login

### Secret access

Current state:

- app-managed secret values stored under envelope encryption (see Security architecture)
- three secret classes: shared (org-readable under policy), personal (owner-keyed, sealed to the user's vault), and app-scope (integration credentials the backend needs without a session)
- personal/shared switching rewraps keys server-side without exposing plaintext, and is restricted to the object's owner
- Key Vault-backed values are fetched on demand and never persisted locally
- reveal, copy, fill, and launch are distinct audited actions

Next target:

- broaden provider coverage beyond Key Vault where needed
- add richer expiry-state visibility without turning the catalog into a second secret store

### Launching

Current state:

- backend returns structured launch payloads and one-time launch tickets
- the web app verifies launcher presence/version through the localhost bridge, then hands off
- the launcher executes RDP and SSH natively per platform: Windows via mstsc with signed profiles and credential handoff, Linux via FreeRDP arguments and terminal-hosted SSH sessions — both with RD Gateway support
- web portal launches open in the browser, with extension-assisted fill where allowed

Target:

- equivalent launcher behavior on macOS

### External integrations

Current state:

- Key Vault adapter with discovery, batch import, manual + automatic sync, and archived/restore views
- app registration discovery/import/sync with owner snapshots, credential expiry metadata, and notification policies (in-app + SMTP email)
- admin-managed Entra and Key Vault runtime configuration; Azure access runs through a dedicated reader identity (workload identity), separate from the OIDC login app
- SMTP notification delivery with a delivery log

Next target:

- richer Entra group-resolution and rights-mapping depth
- selected additional external systems where the workspace adds operational value

## Evolution path

### Stage 1 — done

Category-based monolith with dev auth and simple CRUD.

### Stage 2 — done

Real Azure/Entra identity and group mapping.

### Stage 3 — done

Key Vault-backed secret retrieval and richer secret workflows.

### Stage 4 — partial

Expiry tracking and operational dashboards (app registration credential expiry + notifications shipped; the shared cross-category expiry dashboard is still open).

### Stage 5 — done (Windows launcher + extension)

Launcher helper and browser extension clients; Linux/macOS launcher targets remain.

### Stage 6 — done

Security foundation: envelope encryption at rest, personal vaults with user-managed unlock methods, hardened sessions and perimeter.

## Deployment notes

- Local Docker Compose remains the primary developer workflow.
- The backend and frontend should stay easy to containerize.
- AKS readiness means clear config, health endpoints, and clean service boundaries, not premature service splitting.
