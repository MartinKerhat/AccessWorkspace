# Downloadable artifacts

This directory holds the downloadable build outputs the app serves: the desktop
launcher and the browser-extension packages. The **binaries are not committed**
to git — host them in Azure Blob Storage or GitHub Releases for production. This
folder is kept locally (gitignored) so `docker compose` can bind-mount it for
development.

The backend enumerates files by folder; drop a new build in the right folder and
it appears automatically (old versions are simply the older files in the same
folder). Files are filtered by the extension each folder expects.

## Layout

```
artifacts/
  launcher/
    windows/                 *.exe
    linux/                   *.tar.gz  (binary self-installs on first run)
  extensions/
    chrome/                  *.zip
    firefox/
      signed/                *.xpi   (Mozilla-signed; recommended)
      unsigned/              *.xpi   (developer build; sign via Mozilla before use)
```

## Sources

- **Local (dev):** `ARTIFACTS_SOURCE=local`, `ARTIFACTS_DIR` points at this folder
  (mounted into both the backend and the frontend nginx container). The frontend
  serves the files at `/downloads/...`.
- **Azure Blob (prod):** `ARTIFACTS_SOURCE=blob`, `ARTIFACTS_BLOB_CONTAINER_URL`
  (+ optional `ARTIFACTS_BLOB_SAS`). Blobs use the same folder prefixes as keys;
  downloads point directly at the blob URLs.

Once the extensions are published to the Chrome Web Store / Firefox Add-ons,
set `CHROME_WEB_STORE_URL` / `FIREFOX_EXTENSION_URL` and the app makes the store
listing the primary install action, with direct download as a fallback.
