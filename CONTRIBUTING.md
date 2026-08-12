# Contributing

Thanks for considering a contribution. This is a working internal tool released
as open source, so the bar is practical: changes should be small enough to
review, tested where testable, and documented in the same change.

By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).
Found a security vulnerability? Do **not** open an issue — see
[SECURITY.md](SECURITY.md).

## Getting set up

[docs/development.md](docs/development.md) covers running the app with and
without Docker, seeding demo data, and the test commands. The short version:

```powershell
Copy-Item .env.example .env   # set RESOURCE_SECRET_KEY and POSTGRES_PASSWORD
docker compose up --build
```

## Before you open a pull request

- `go build ./...` and `go test ./...` from `backend`
- `npx tsc --noEmit` and `npm run build` from `frontend`
- `gofmt` your Go changes — but only the files you touched
- update [docs/](docs/) in the same change if behavior changed

## Conventions that matter here

**Documentation ships with the feature.** [`docs/`](docs/) describes what the app
does today; if your change alters behavior, the matching document changes in the
same pull request. Keep each document answering the one question it is listed
under, and do not add roadmaps or plans for unbuilt work — a feature gets
documented once it exists.

**Bump the version when you change the launcher or the extension.** Both are
released by CI on a version bump, so a code change without one publishes
nothing:

- launcher: `Version` in `launcher/internal/launcherinfo/launcherinfo.go`
- extension: `version` in **both** `browser-extension/chrome/manifest.json` and
  `browser-extension/firefox/manifest.json`

Backend and frontend images build from every push and need no version bump.

**Migrations are append-only and run once.** Add a new numbered file in
`backend/migrations/`; never edit one that has shipped, because applied versions
are recorded and will not re-run.

**Secrets never enter the repository.** No credentials, vault names, tenant ids,
host names, or real screenshots in code, tests, fixtures, or documentation.
`.env` is gitignored and stays that way.

**Comments explain why, not what.** Prefer a sentence about the constraint that
forced an unusual approach over a restatement of the code. If a workaround
exists because of a platform quirk, say which quirk.

## Commit messages

Short, imperative, prefixed by kind — matching the existing history:

```
feat: add expiry reminders for key vault secrets
fix: closing gap for RDP connections that refuse saved credentials
docs: rework readme as product presentation
build(deps): bump postcss
```

## Reporting bugs and proposing features

Open an issue. For a bug, the useful ones say what you did, what happened, what
you expected, and which part of the system was involved (backend, frontend,
launcher, extension) — plus the version if it is a launcher or extension issue.

For a feature, describe the operational problem first. This tool is opinionated
about scope: it is an access workspace, not a full privileged-access management
platform, and it deliberately does not do secret rotation orchestration or
approval workflows. A proposal that explains the workflow it fixes is much
easier to evaluate than one that names a feature.

## Licensing

Contributions are accepted under the [MIT License](LICENSE), the same license
the project is released under.
