# Product Vision

## Purpose

Access Workspace is an internal operational access platform for discovering, using, and governing shared access in a structured and auditable way.

The long-term goal is to replace informal access sharing and reduce dependence on disconnected tools such as chat messages, ad hoc spreadsheets, Azure Portal navigation, and platform-specific connection managers.

## Core problems to solve

- Shared operational access is spread across people, chats, docs, and personal tooling.
- Users often do not know what shared access exists or who owns it.
- Secrets are revealed and shared informally.
- Azure Key Vault is powerful but inefficient for daily operational use.
- Shared website logins and portal access are not centrally organized.
- Expiring secrets and app credentials are easy to miss.
- RoyalTS-style connection workflows are not cleanly cross-platform.

## Product goal

Create one internal workspace where authorized users can:

- enter a rights-aware workspace after sign-in
- see only the object categories relevant to their access
- work with operational objects in dedicated category views instead of one mixed catalog
- launch or inspect connections
- work with Azure Key Vault through a better UI
- manage app registrations and track credential risk
- reveal or copy shared passwords when policy allows it
- monitor sensitive activity and later expiration workflows

## Target users

- operations and infrastructure engineers
- support engineers
- platform engineers
- administrators managing shared internal access
- internal users who need approved access to shared tools and environments

## Product principles

- Metadata first, secret second: users should discover and understand access before revealing secret material.
- Azure is the authority: identity and group membership should come from Azure/Entra.
- The app should have a clear auth entry experience: even if the target flow becomes Microsoft SSO, the product should feel like an authenticated workspace rather than an always-open internal page.
- Navigation should be permission-aware: users should only see categories they are allowed to use.
- Categories should be based on object structure and workflow, not one mixed catalog with heavy filtering.
- Least privilege by design: viewing, revealing, launching, and filling are separate capabilities.
- Operational usability matters: the UI should optimize for fast daily work, not raw record browsing.
- The current Key Vault section is the reference UI language: its picker behavior, button treatment, modal structure, and text-field styling should be treated as the baseline for future interface consistency across the workspace.
- QA/debugging surfaces should stay temporary: low-level authorization details may be shown during testing, but should later move out of the everyday user account UI into a more suitable diagnostics or admin surface.
- Audit sensitive actions: reveal, copy, launch, and later autofill should be logged.
- Cross-platform is a requirement: the future launcher path must work across Windows, macOS, and Linux.

## Product pillars

### 1. Connection workspace

Manage and use shared SSH and RDP access with a future cross-platform launcher/helper. This should eventually replace RoyalTS-like workflows with a backend-driven and policy-aware approach.

### 2. Key Vault operations

Provide a better operational UI for Azure Key Vault, starting with secrets today but leaving room for future Key Vault object growth.

### 3. Password management

Support shared login/password style entries for websites, tools, and legacy systems. The first stage is reveal/copy and contextual usage.

### 4. App registration visibility

Represent application registrations and related credentials as first-class operational objects.

### 5. Expiry visibility

Track expiring client secrets, app registration credentials, and similar access-related lifecycle risks. The goal is reminders and visibility, not full rotation orchestration.

### 6. Azure-native authorization

Use Azure/Entra identities and groups as the source of truth for who can see and use resources.

## Scope boundaries

### In scope

- category-based workspace views
- resource detail and action flows
- audit logging
- Azure/Entra-backed identity and group mapping
- Azure Key Vault operational UI
- shared password entries
- expiry reminders and ownership visibility
- future launcher helper for SSH and RDP
- future browser extension for portal autofill

### Out of scope for now

- full privileged access management platform behavior
- secret rotation orchestration
- approval workflows
- multi-tenant SaaS design
- microservices decomposition
- deep native browser automation in the web app itself

## Success criteria

- users stop relying on informal password sharing for common operational access
- teams can quickly discover who owns a resource and whether they are allowed to use it
- users see only the categories and objects relevant to their rights
- access actions are visible and auditable
- Azure Key Vault becomes easier to browse and use safely
- expiring app and secret risks are surfaced early
- operational connection workflows move toward one cross-platform system
