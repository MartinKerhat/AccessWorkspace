# Browser Extension Distribution

Current browser-extension packaging is split into three channels:

- Chrome / Edge:
  - Package artifact: `frontend/public/downloads/access-workspace-browser-extension-chrome-v0.2.5.zip`
  - Current repo-ready flow: package build plus optional private Chrome Web Store publishing script
  - Production path: private Chrome Web Store listing or managed enterprise deployment
- Firefox:
  - Signed artifact currently bundled: `frontend/public/downloads/access-workspace-browser-extension-firefox-signed-v0.2.5.xpi`
  - Unsigned repo-built artifact: `frontend/public/downloads/access-workspace-browser-extension-firefox-v0.2.5.xpi`
  - Current repo-ready flow: Mozilla-approved signed XPI downloaded from Developer Hub and re-served by the app
  - Production path: signed self-distributed XPI or later a listing URL

The repo-hosted current browser-extension build is now `0.2.5`, including the Mozilla-signed Firefox XPI.
- Safari:
  - Current repo-ready flow: not packaged yet
  - Remaining production work: Safari Web Extension wrapper app, Apple signing, and notarization

## What the app now supports

- Browser-extension runtime metadata is exposed by the backend, including per-browser package status
- Docker frontend builds validate the active download artifacts and remove older files before `public/downloads` is copied into the final static site
- The web app has a browser-extension manager so users can:
  - see which browser packages exist
  - open a real store/listing install URL when configured
  - fall back to package download when store/listing distribution is not configured yet
  - connect the installed extension to the workspace
- The extension itself now uses browser-neutral copy instead of Chrome-only wording

## Environment variables

- `CHROME_WEB_STORE_URL`
  - Optional install URL for the private or unlisted Chrome Web Store listing.
  - When set, the app shows `Install from Chrome Web Store` instead of ZIP download for Chromium browsers.
- `FIREFOX_EXTENSION_URL`
  - Optional install URL for a signed Firefox add-on listing or signed self-distribution URL.
  - When set, the app prefers that install URL. Otherwise it now falls back to the locally hosted signed XPI if present.

## Chrome private Web Store steps

1. Create or use a Chrome Web Store developer account.
2. Make sure the publisher Google account has 2-step verification enabled.
3. Prepare the first listing in the Chrome Web Store developer dashboard:
   - upload the extension package
   - complete the listing tab
   - complete the privacy tab
   - set visibility to `Private`
   - add trusted testers or Google Groups
4. Copy the final Web Store listing URL into `CHROME_WEB_STORE_URL`.
5. After the initial listing exists, future updates can use:
   - `scripts/publish-chrome-webstore.ps1`

Required environment variables for the publish script:

- `CHROME_WEB_STORE_CLIENT_ID`
- `CHROME_WEB_STORE_CLIENT_SECRET`
- `CHROME_WEB_STORE_REFRESH_TOKEN`
- `CHROME_WEB_STORE_PUBLISHER_ID`
- `CHROME_WEB_STORE_EXTENSION_ID`

Example:

```powershell
.\scripts\publish-chrome-webstore.ps1 -Publish
```

## Firefox signing steps

Firefox release and beta require Mozilla signing. An unsigned XPI is expected to fail in normal Firefox even if the archive itself is structured correctly.

Required environment variables for signing:

- `FIREFOX_AMO_JWT_ISSUER`
- `FIREFOX_AMO_JWT_SECRET`

Build a fresh XPI:

```powershell
.\scripts\build-chrome-zip.ps1
.\scripts\build-firefox-xpi.ps1
```

Sign it through Mozilla:

```powershell
.\scripts\sign-firefox-addon.ps1
```

If you download the approved signed XPI from Mozilla Developer Hub, place that signed artifact into:

- `frontend/public/downloads/`

The current bundled signed Firefox artifact in this repo is:

- `access-workspace-browser-extension-firefox-signed-v0.2.5.xpi`

The new unsigned Firefox artifact that should be signed next is:

- `access-workspace-browser-extension-firefox-v0.2.5.xpi`

If later you receive a stable listing URL or update URL, set:

- `FIREFOX_EXTENSION_URL`

## What still cannot be solved only in this repo

- The first Chrome Web Store listing still needs a real publisher account and dashboard completion
- Firefox signing still needs real Mozilla API credentials
- Trusted install in Safari still requires Apple’s native wrapper and signing flow

## Active Docker Download Set

The frontend Docker image should expose only these current distributables:

- `access-workspace-browser-extension-chrome-v0.2.5.zip`
- `access-workspace-browser-extension-firefox-v0.2.5.xpi`
- `access-workspace-browser-extension-firefox-signed-v0.2.5.xpi`
- `access-workspace-launcher-windows-amd64-v0.5.6.exe`

If any of these files are missing, the frontend Docker build fails instead of producing broken download links.

## Recommended production sequence

1. Finish the first private Chrome Web Store listing and put its URL into `CHROME_WEB_STORE_URL`.
2. Keep the Mozilla-signed Firefox XPI in app downloads, or set `FIREFOX_EXTENSION_URL` when you have a stable install URL.
3. Build the Safari wrapper only after the browser-extension UX and backend contract settle.
