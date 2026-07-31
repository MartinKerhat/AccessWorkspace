# Browser Extension Distribution

The extension ships per browser from `browser-extension/{chrome,firefox}`,
packaged into `artifacts/extensions/` (see `artifacts/README.md` for how the
backend serves artifacts locally vs from Azure Blob in production).

- **Chrome / Edge / other Chromium**
  - Package: `artifacts/extensions/chrome/access-workspace-browser-extension-chrome-v<version>.zip`
  - Production path: **unlisted Chrome Web Store listing** — installable via
    link from the app, not searchable. Direct ZIP download stays as the
    developer fallback.
- **Firefox**
  - Release Firefox refuses unsigned XPIs, so builds are signed through
    Mozilla (AMO unlisted channel) and self-distributed:
    `artifacts/extensions/firefox/signed/…-signed-v<version>.xpi`
  - The AMO add-on id is `browser-fill@access-workspace.local` and must not
    change — it is the add-on's permanent identity from earlier signings.
- **Safari**
  - Not packaged yet; needs Apple's Web Extension wrapper app, signing, and
    notarization.

## How the app decides what to offer

The backend exposes per-browser package status; the web app's Browser
Extensions modal shows a store install link when configured, otherwise the
newest signed/zip artifact:

- `CHROME_WEB_STORE_URL` — when set, Chromium browsers get
  "Install from Chrome Web Store" as the primary action.
- `FIREFOX_EXTENSION_URL` — optional override; otherwise the app serves the
  newest locally hosted signed XPI.

## Releasing a new version

This repository is the product, not any particular deployment — CI publishes
only to neutral distribution points and never pushes into a deployment.

Bump `version` in BOTH manifests (they must match) and merge to main —
`.github/workflows/release-extension.yml` then builds the ZIP, uploads and
submits it to the Chrome Web Store, signs the Firefox XPI through Mozilla,
and attaches both files to a GitHub Release tagged `extension-v<version>`.
Pushes that change extension code without a version bump release nothing
(stores reject same-version re-uploads). Repository secrets:
`CHROME_WEB_STORE_CLIENT_ID`, `CHROME_WEB_STORE_CLIENT_SECRET`,
`CHROME_WEB_STORE_REFRESH_TOKEN`, `CHROME_WEB_STORE_EXTENSION_ID`,
`FIREFOX_AMO_JWT_ISSUER`, `FIREFOX_AMO_JWT_SECRET` — missing secrets skip
that publish step with a warning instead of failing.

## How deployments pick releases up

Each deployment chooses its artifact source (`artifacts/README.md`):

- `ARTIFACTS_SOURCE=github` + `ARTIFACTS_GITHUB_REPO=<owner>/<repo>` — the
  backend lists this repo's GitHub Releases (cached, optional
  `ARTIFACTS_GITHUB_TOKEN` for rate limits) and serves the newest published
  builds automatically. No per-deployment steps when a new version ships.
- `ARTIFACTS_SOURCE=blob` / `local` — operators copy the release assets into
  their own store; the app picks up whatever is newest there.

The GitHub Actions workflow is the only release path — there are no local
build/publish scripts for the extension. The two helper scripts that remain:

```powershell
.\scripts\build-extension-icons.ps1              # regenerate the committed icon set
.\scripts\get-chrome-webstore-refresh-token.ps1  # (re)issue the CI OAuth refresh token
```

The desktop launcher releases the same way: bumping `Version` in
`launcher/internal/launcherinfo/launcherinfo.go` on main triggers
`.github/workflows/release-launcher.yml`, which builds the Windows and Linux
binaries and attaches them to a `launcher-v<version>` GitHub Release
(`launcher/build.ps1` stays as the local dev build).

## Store listing requirements

- Privacy policy: `docs/browser-extension-privacy.md` (link its public URL in
  the dashboard's privacy tab).
- The manifest ships icons (`browser-extension/*/icons/`); the 128 px PNG
  doubles as the store icon.
- The extension's `<all_urls>` access exists because fillable portals are
  defined per deployment at runtime; the single-purpose statement and
  permission justifications are prepared in
  `.dev-notes/browser-extension-store-plan.md`.

## What still cannot be solved in this repo

- The first Chrome Web Store listing needs a real publisher account and a
  one-time manual dashboard submission.
- Firefox signing needs Mozilla API credentials.
- Safari needs Apple's native wrapper and signing flow.
