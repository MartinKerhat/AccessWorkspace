# Browser Extension Distribution

**Audience:** maintainers releasing extension builds. **Covers:** the release
path that exists today.

The extension ships per browser from `browser-extension/{chrome,firefox,safari}`,
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
  - Safari only runs extensions that ship inside a Mac app, so the extension
    is wrapped by the Xcode project in `browser-extension/safari-app`
    (generated with Apple's `safari-web-extension-converter`, referencing
    the `browser-extension/safari` sources). CI builds it on a Mac runner,
    signs it with the Developer ID, notarizes it and attaches
    `artifacts/extensions/safari/…-safari-v<version>.dmg` to the release.
    Users drag the app to Applications, open it once and turn the extension
    on in Safari Settings → Extensions.
  - Mac App Store (Safari's own extension store): the same job also archives
    the app with the "Apple Distribution" / "Mac Installer Distribution"
    certificates and uploads it to App Store Connect
    (`scripts/build-safari-extension.sh --app-store`, secrets
    `APPLE_APP_STORE_P12` / `APPLE_APP_STORE_P12_PASSWORD`; skipped when
    missing). The two Mac App Store provisioning profiles are created and
    refreshed by `scripts/asc-profiles.py` through the App Store Connect API
    at build time, so no profile is stored anywhere and a renewed certificate
    just works. Each uploaded version is submitted for Apple's review in App
    Store Connect — a day or three — while the notarized `.dmg` is available
    immediately. Set `SAFARI_EXTENSION_URL` to the App Store listing to make
    it the primary action, like the other stores.

## How the app decides what to offer

The backend exposes per-browser package status; the web app's Browser
Extensions modal shows a store install link when configured, otherwise the
newest signed/zip artifact:

- `CHROME_WEB_STORE_URL` — when set, Chromium browsers get
  "Install from Chrome Web Store" as the primary action.
- `FIREFOX_EXTENSION_URL` — optional override; otherwise the app serves the
  newest locally hosted signed XPI.
- `SAFARI_EXTENSION_URL` — optional Mac App Store listing; otherwise the app
  serves the newest notarized Safari `.dmg`.

## Releasing a new version

This repository is the product, not any particular deployment — CI publishes
only to neutral distribution points and never pushes into a deployment.

Bump `version` in ALL THREE manifests (they must match) and merge to main —
`.github/workflows/release-extension.yml` then builds the ZIP, uploads and
submits it to the Chrome Web Store, signs the Firefox XPI through Mozilla,
builds, signs and notarizes the Safari app on a Mac runner, and attaches the
files to a GitHub Release tagged `extension-v<version>`.
Pushes that change extension code without a version bump release nothing
(stores reject same-version re-uploads). Repository secrets:
`CHROME_WEB_STORE_CLIENT_ID`, `CHROME_WEB_STORE_CLIENT_SECRET`,
`CHROME_WEB_STORE_REFRESH_TOKEN`, `CHROME_WEB_STORE_EXTENSION_ID`,
`FIREFOX_AMO_JWT_ISSUER`, `FIREFOX_AMO_JWT_SECRET`, and for Safari the same
Apple secrets the launcher uses (`APPLE_DEVELOPER_ID_P12`,
`APPLE_DEVELOPER_ID_P12_PASSWORD`, `APPLE_API_KEY_ID`, `APPLE_API_ISSUER_ID`,
`APPLE_API_KEY_P8`) — missing secrets skip that publish step with a warning
instead of failing.

## How deployments pick releases up

Each deployment chooses its artifact source (`artifacts/README.md`):

- `ARTIFACTS_SOURCE=github` + `ARTIFACTS_GITHUB_REPO=<owner>/<repo>` — the
  backend lists this repo's GitHub Releases (cached, optional
  `ARTIFACTS_GITHUB_TOKEN` for rate limits) and serves the newest published
  builds automatically. No per-deployment steps when a new version ships.
- `ARTIFACTS_SOURCE=blob` / `local` — operators copy the release assets into
  their own store; the app picks up whatever is newest there.

The GitHub Actions workflow is the only release path — there are no local
publish scripts for the extension. The helper scripts that remain:

```powershell
.\scripts\build-extension-icons.ps1              # regenerate the committed icon set
.\scripts\get-chrome-webstore-refresh-token.ps1  # (re)issue the CI OAuth refresh token
```

```sh
scripts/build-macos-icons.sh          # macOS icon sets (launcher .icns, Safari app icon) from the same design
scripts/build-safari-extension.sh     # local Safari build (the same script CI runs; unsigned without a Developer ID)
```

The desktop launcher releases the same way: bumping `Version` in
`launcher/internal/launcherinfo/launcherinfo.go` on main triggers
`.github/workflows/release-launcher.yml`, which builds the Windows and Linux
binaries, then the signed and notarized macOS `.dmg` on a Mac runner, and
attaches them to a `launcher-v<version>` GitHub Release (`launcher/build.ps1`
and `launcher/scripts/build_macos.sh` stay as the local dev builds).

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
- The first Mac App Store listing likewise: an app record for the bundle id
  `io.github.martinkerhat.accessworkspace.browserfill` in App Store Connect
  with description, screenshots and the privacy policy URL, and a manual
  "Submit for Review" of the first uploaded build.
- Firefox signing needs Mozilla API credentials.
- Safari and the macOS launcher need an Apple Developer Program membership:
  a "Developer ID Application" certificate (exported as .p12) and an App
  Store Connect API key for notarization, stored as the repository secrets
  above.
