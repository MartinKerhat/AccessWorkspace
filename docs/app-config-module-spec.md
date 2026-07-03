# App Config Module Specification

## Purpose

This document defines the next major workspace module for application configuration management.

Working name:

- `App Configs`

Primary goal:

- let teams model, review, validate, and generate application configuration in one operational UI instead of maintaining scattered YAML, ConfigMap, and ExternalSecret files by hand

This module should complement the current `Key Vault` area rather than replace it.

## Problem statement

Current deployment configuration for one app is often split across:

- base `ConfigMap` files
- environment-specific `ConfigMap` files
- base `ExternalSecret` files
- environment-specific `ExternalSecret` files
- component-specific files such as `backend`, `frontend`, `mailbox`, and jobs
- placeholder values such as `>> DEFINE IN ENVS <<`

This creates several recurring problems:

- users must mentally merge multiple files to understand one app in one environment
- adding one variable often requires edits in several places
- missing values in one environment are easy to miss
- secret and non-secret values are managed through separate workflows
- access separation is poor when teams should only manage selected apps or environments
- rollout behavior is hard to reason about because desired config and deployed config are not represented as one coherent domain object

## Product direction

The workspace should gain a dedicated application configuration module that becomes the control plane for:

- app-level config structure
- environment separation
- component separation
- secret versus non-secret handling
- validation and completeness checks
- resolved environment views
- generated deployment outputs

The module should be centered on the question:

- "What configuration does this app component effectively have in this environment?"

## Relationship to current Key Vault behavior

The current `Key Vault` module should remain valid and largely unchanged.

Key rules:

- `Key Vault` remains the operational surface for Azure Key Vault-backed discovery, import, sync, metadata, and reveal
- the new `App Configs` module may reference Key Vault-managed secrets without taking over the Key Vault category itself
- app configuration entries may be app-managed or externally referenced depending on the use case

This preserves the current design rule from the workspace:

- external systems remain systems of record when that is appropriate
- the workspace may still own app-authored config structure, policy, and deployment outputs

## Core user outcomes

Users should be able to:

- open one app and environment and see the full effective config
- understand which values are inherited, overridden, missing, secret, or externally referenced
- manage config entries in one structured UI instead of raw YAML
- restrict access by app, component, environment, and action
- generate deployment outputs consistently
- detect missing config before deployment

## Non-goals for the first iteration

- replacing Argo CD
- replacing Kubernetes as runtime
- replacing Azure Key Vault as the only supported secret backend on day one
- implementing direct in-cluster hot reload behavior for every workload
- supporting every possible config file format from the beginning

## Scope overview

The module should model configuration across these dimensions:

- `application`
- `component`
- `environment`
- `config entry`
- `value source`
- `resolution layer`

Example:

- application: `insio-next`
- components: `backend`, `frontend`, `mailbox`, `background-jobs`
- environments: `ci`, `staging`, `preprod`, `production`

## Domain concepts

### Application

Represents one deployable product or service family.

Examples:

- `insio-next`
- `billing-api`
- `portal-ui`

Core fields:

- `id`
- `name`
- `slug`
- `description`
- `owner`
- `owner_team`
- `allowed_groups`
- `status`
- `created_at`
- `updated_at`

### Component

Represents one deployable or logically separate workload inside an application.

Examples:

- `backend`
- `frontend`
- `mailbox`
- `worker`
- `storybook`

Reason:

- one application may intentionally avoid sharing all secrets across all workloads
- component-level config separation is a first-class requirement in the current deployment examples

Core fields:

- `id`
- `application_id`
- `name`
- `slug`
- `description`
- `runtime_kind` such as `deployment`, `job`, `cronjob`, `service-only`
- `allowed_groups`
- `status`

### Environment

Represents a deployment target or configuration context.

Examples:

- `ci`
- `dev`
- `staging`
- `preprod`
- `production`

Core fields:

- `id`
- `name`
- `slug`
- `description`
- `sensitivity_level`
- `sort_order`
- `status`

### Config contract

Represents the expected variables for an application or component.

Purpose:

- define which keys should exist
- define whether values are required
- define whether a key is secret or plain
- define whether a key applies to all environments or only selected ones

Examples:

- `DATABASE_URL` required for `backend`
- `WEB_APP_URL` required for `frontend`
- `SMTP_PASSWORD` required only when mail is enabled

Core fields:

- `id`
- `application_id`
- `component_id` optional
- `key`
- `description`
- `value_kind`
- `required`
- `environment_scope`
- `default_source_mode`
- `validation_rules`
- `display_group`

### Config entry

Represents one managed configuration item.

This is the central editable object.

Core fields:

- `id`
- `application_id`
- `component_id` optional
- `environment_id` optional for base/shared values
- `key`
- `value_kind`
- `source_mode`
- `plain_value` when non-secret and app-managed
- `secret_value` encrypted when secret and app-managed
- `external_provider`
- `external_reference`
- `inheritance_mode`
- `notes`
- `owner`
- `allowed_groups`
- `created_at`
- `updated_at`
- `archived_at`

### Value kinds

- `plain`
- `secret`
- `json`
- `multiline` later if needed

### Source modes

- `app_managed`
- `key_vault_reference`
- `external_reference`

Future room:

- `hashicorp_vault_reference`
- `app_configuration_reference`

### Resolution layer

The module should support layered resolution rather than only a flat per-environment blob.

Initial layers:

- `shared_base`
- `application_base`
- `environment_override`
- `component_override`

This allows one final effective value to be built from a predictable precedence order.

## Resolution model

The system should build a final effective config for:

- one application
- one component
- one environment

Suggested precedence from lower to higher:

1. shared base
2. application base
3. environment override
4. component-specific override
5. component plus environment override

Resolution result per key should include:

- `effective key`
- `effective value` or masked value
- `status`
- `source layer`
- `source object id`
- `value kind`
- `source mode`
- `is_inherited`
- `is_overridden`
- `is_missing`
- `last_changed_by`
- `last_changed_at`

## Resolved environment view

This should be the main screen of the module, not a secondary filter.

Primary entry point:

- choose `application`
- choose `environment`
- choose `component`

Then show the full effective configuration for that target.

Recommended columns:

- `key`
- `effective value`
- `type`
- `source`
- `status`
- `last changed by`
- `last changed at`

Recommended statuses:

- `ok`
- `missing`
- `inherited`
- `overridden`
- `conflict`
- `external reference`

Secret behavior:

- secret values masked by default
- reveal allowed only for users with explicit permission
- completeness checks should still include masked secret entries

This view should answer:

- what variables this target effectively has
- where they came from
- what is missing
- what differs from another environment or component

## Expected UI areas

### 1. Applications list

Purpose:

- browse managed applications
- see ownership and environment health at a glance

Useful summary fields:

- component count
- environment count
- missing-value count
- secret count
- last deployment-output change

### 2. Application overview

Purpose:

- show application structure
- list components
- list environments
- show config health summary

### 3. Resolved config page

Purpose:

- show the final effective env/config for one app, one environment, one component

Suggested tabs:

- `Resolved`
- `Sources`
- `Diff`
- `Validation`

### 4. Config contract page

Purpose:

- define expected keys and rules
- reduce "forgot one env" errors

Show:

- required keys
- optional keys
- secret/plain type
- environment applicability
- component applicability

### 5. Entry editor

Purpose:

- create or update individual config entries
- choose source mode
- choose inheritance scope
- validate collisions and missing requirements

### 6. Deployment outputs page

Purpose:

- preview generated artifacts
- compare desired output against exported/deployed forms

## Access model

This module should support more granular access than the current Key Vault-only pattern.

Recommended access dimensions:

- `application access`
- `component access`
- `environment access`
- `action access`
- `secret visibility`

Suggested actions:

- `view`
- `edit`
- `reveal`
- `export`
- `deploy` later

Examples:

- a developer may edit `insio-next` only
- a team may edit `backend` and `frontend` but not `mailbox`
- a user may edit `staging` and `preprod` but not `production`
- a user may change a secret reference without being allowed to reveal the current live value

## Secret handling rules

The module should support a unified editing experience while preserving safe storage distinctions.

Rules:

- non-secret app-managed values may be stored directly
- secret app-managed values must be encrypted at rest
- external secret references should store only provider metadata and reference identifiers
- secret reveal must be audited
- secret updates must be audited

The UI should still present secret and non-secret entries together in the resolved environment view so users see one complete configuration picture.

## Import from current YAML structure

The module should be able to import the current GitOps-oriented configuration layout.

Expected import sources:

- `ConfigMap` YAML
- `ExternalSecret` YAML
- selected app-level config helper files where useful

Import goals:

- infer application from path
- infer component from filename or manifest name
- infer environment from folder path
- convert plain values into app config entries
- convert external secret mappings into referenced secret entries
- preserve provenance for later traceability

Useful provenance metadata:

- original file path
- original manifest kind
- original manifest name
- import timestamp
- imported by user

This allows gradual adoption instead of requiring greenfield authoring.

## Validation rules

The module should detect and surface:

- required keys missing for a target environment
- conflicting duplicate definitions at the same precedence layer
- secret/plain type mismatches
- keys defined for environments where they are not expected
- references to missing external provider objects
- entries present in deployment output but absent from the config contract
- stale placeholders such as `>> DEFINE IN ENVS <<`

Validation should be visible both:

- in edit flows
- on the resolved config page

## Generated outputs

The module should generate deployment-friendly outputs from the normalized model.

Initial output targets:

- `.env` preview
- Kubernetes `ConfigMap` manifest
- Kubernetes `Secret` manifest
- Kubernetes `ExternalSecret` manifest

Future targets:

- Helm values fragments
- Azure App Configuration import payloads
- selected JSON config payloads

Key rule:

- generated outputs are projections of the managed config model, not the source of truth themselves

## Deployment boundary and GitOps model

Recommended first delivery model:

- this app owns the configuration model
- this app generates deployment artifacts
- Git remains the deployable source of truth
- Argo CD continues to reconcile the cluster

This means the first version should prefer:

- `App Configs -> generated manifests -> GitOps repo -> Argo CD -> AKS`

This is safer than making the workspace write directly to Kubernetes on day one.

Future room:

- direct sync to Azure App Configuration
- direct sync to Kubernetes

## External Secrets and ConfigMaps

This module may reduce or eliminate hand-authored `ExternalSecret` and `ConfigMap` manifests over time.

However:

- removing `ExternalSecret` objects does not remove the need for a system that owns final Kubernetes `Secret` objects
- changing `ConfigMap` or `Secret` data does not automatically guarantee workload rollout behavior
- env-var-driven workloads still need predictable restart or rollout handling when values change

So the real objective is:

- make the workspace the configuration control plane

Not merely:

- remove one operator

## Auditing

The module should create audit events for:

- config contract create/update/archive
- config entry create/update/archive
- secret reveal
- generated output export
- environment diff view if considered sensitive

Useful metadata:

- application
- component
- environment
- key
- source mode
- old versus new status where safe

## Example: insio-next

### Structure

- application: `insio-next`
- components:
  - `backend`
  - `frontend`
  - `mailbox`
  - `background-jobs`
- environments:
  - `ci`
  - `staging`
  - `preprod`
  - `production`

### Example contract entries

- `DATABASE_URL` required for `backend`
- `REDIS_URL` required for `backend`
- `APP_BASE_URL` required for `backend` and `frontend`
- `WEB_APP_URL` required for `frontend`
- `SMTP_PASSWORD` required for `mailbox`
- `OTEL_ENDPOINT` required for all runtime components
- `SESSION_SECRET` required for `backend`

### Example resolved view question

For:

- app `insio-next`
- environment `preprod`
- component `backend`

The resolved page should answer:

- which keys exist
- which are inherited from base
- which are overridden in preprod
- which are secret references
- which required keys are missing

## MVP proposal

### MVP 1

Deliver:

- application, component, and environment models
- config contract
- config entries with plain and secret support
- Key Vault reference mode
- resolved config page
- validation for missing required keys
- import from current YAML manifests

Do not require yet:

- direct Kubernetes writes
- direct Azure App Configuration sync
- full deployment automation

### MVP 2

Deliver:

- generated `ConfigMap` and `ExternalSecret` outputs
- diff views between environments
- stronger permissions by app and environment
- rollout/restart metadata support

### MVP 3

Deliver:

- GitOps export workflow
- optional direct sync targets
- richer provider support
- deployment status visibility

## Suggested implementation notes

Backend direction:

- keep this as a new category-specific module inside the modular monolith
- separate config contract objects from resolved-value projections
- reuse current encryption and audit patterns where appropriate

Frontend direction:

- make resolved config the primary page
- use tables with strong status indicators and inline provenance
- avoid forcing users to navigate to separate secret and plain-config worlds for one target environment

Persistence direction:

- store normalized config entries, not only generated YAML blobs
- keep provenance metadata for imported entries
- treat generated artifacts as outputs, not primary records

## Summary

`App Configs` should become the workspace area where teams can:

- understand one app's effective configuration
- manage plain and secret values in one structured workflow
- separate access by app, component, and environment
- validate completeness before deployment
- generate Kubernetes and related outputs consistently

The core success criterion is simple:

- a user should be able to open one application, one environment, and one component and immediately understand what configuration exists, what is missing, and what should be changed without manually merging YAML files.
