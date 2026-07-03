# Architecture

## High-level approach

Keep deployment simple while designing clear internal boundaries for future growth.

Current deployment target:

- one backend service
- one frontend application
- one PostgreSQL database

Future optional clients:

- local launcher/helper for SSH and RDP
- browser extension for portal autofill

These should integrate with the same backend rather than becoming separate platforms.

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

Planned responsibilities:

- receive approved launch payloads from the backend or web app
- execute cross-platform SSH and RDP launches
- handle OS-specific launch behavior
- preserve a minimal local integration layer without owning central access policy

This component is required for a serious RoyalTS replacement path.

### Browser extension

Planned responsibilities:

- identify supported login pages
- request approved portal credentials from backend flows
- assist with field fill on allowed websites
- log sensitive fill actions

This should remain separate from the main web UI.

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
- login entry flow and session bootstrap
- Microsoft sign-in start/callback path
- backend authorization based on resolved category capabilities and roles

Next target:

- broader Entra group resolution hardening
- cleaner separation of QA-only auth diagnostics from the main account UI

### Secret access

Current state:

- inline secret mode for development/demo
- external reference placeholders
- secret providers fetch values on demand
- Key Vault provider is the first real provider
- reveal and related actions are audited

Next target:

- broaden provider coverage beyond Key Vault where needed
- add richer expiry-state visibility without turning the catalog into a second secret store

### Launching

Near term:

- backend returns structured launch payloads

Target:

- local helper receives and executes approved launch instructions
- helper supports SSH first, then RDP

### External integrations

Current state:

- manual records plus placeholders for several categories
- Key Vault adapter and automatic sync job
- admin-managed Entra and Key Vault runtime configuration

Next target:

- app registration integration
- richer Entra group-resolution and rights-mapping depth
- selected additional external systems where the workspace adds operational value

## Evolution path

### Stage 1

Category-based monolith with dev auth and simple CRUD.

### Stage 2

Real Azure/Entra identity and group mapping.

### Stage 3

Key Vault-backed secret retrieval and richer secret workflows.

### Stage 4

Expiry tracking and operational dashboards.

### Stage 5

Launcher helper and browser extension clients.

## Deployment notes

- Local Docker Compose remains the primary developer workflow.
- The backend and frontend should stay easy to containerize.
- AKS readiness means clear config, health endpoints, and clean service boundaries, not premature service splitting.
