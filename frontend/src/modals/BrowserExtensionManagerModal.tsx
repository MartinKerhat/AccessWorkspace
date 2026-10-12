import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { BrowserExtensionPackage } from "../types";
import { formatArtifactMeta } from "../format";
import { browserExtensionStatusLabel } from "../browserExtension";
import { scrimDismissProps } from "./scrim";

type Props = {
  visiblePackages: BrowserExtensionPackage[];
  currentPackage: BrowserExtensionPackage | null;
  busy: boolean;
  // True when the user holds generator.passwords; the opt-in is hidden otherwise.
  canSuggestPasswords: boolean;
  onMessage: (message: string | undefined) => void;
  onClose: () => void;
  onConnect: () => void;
};

export function BrowserExtensionManagerModal({ visiblePackages, currentPackage, busy, canSuggestPasswords, onMessage, onClose, onConnect }: Props) {
  // Opt-in for generated passwords on sign-up forms. Stored with the user's
  // generator settings (part "extension"), which the extension reads too, so
  // the switch applies to every browser the user has connected.
  const [suggestPasswords, setSuggestPasswords] = useState<boolean | null>(null);
  useEffect(() => {
    if (!canSuggestPasswords) {
      return;
    }
    void api
      .generatorPreferences()
      .then(({ preferences }) => setSuggestPasswords(preferences.extension?.suggestPasswords === true))
      .catch(() => setSuggestPasswords(false));
  }, [canSuggestPasswords]);

  async function toggleSuggestPasswords(next: boolean) {
    setSuggestPasswords(next);
    try {
      await api.saveGeneratorPreference("extension", { suggestPasswords: next });
      onMessage(next ? "The extension will offer generated passwords on sign-up forms." : "The extension will no longer offer generated passwords.");
    } catch (error) {
      setSuggestPasswords(!next);
      onMessage(error instanceof Error ? error.message : "Saving the setting failed");
    }
  }

  return (
    <div className="modal-scrim" {...scrimDismissProps(onClose)}>
      <div className="modal-card browser-extension-manager-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <p className="eyebrow">Browser extensions</p>
            <h2>Install and connect</h2>
          </div>
          <button className="button ghost" onClick={onClose}>
            Close
          </button>
        </div>
        <div className="detail-section">
          <p className="detail-description">
            Install the package for your browser, then connect this browser once from the web app. After that handshake, the extension
            keeps its own long-lived workspace session.
          </p>
          <p className="detail-description">
            Current browser: {currentPackage?.label || "Unknown browser"}
            {currentPackage ? ` (${browserExtensionStatusLabel(currentPackage.status)})` : ""}
          </p>
        </div>
        <div className="action-row compact-actions">
          <button className="button primary" disabled={busy} onClick={onConnect}>
            Connect this browser
          </button>
          {currentPackage?.installUrl ? (
            <a className="button secondary launch-link-button" href={currentPackage.installUrl} target="_blank" rel="noreferrer">
              {currentPackage.actionLabel || "Install current browser add-on"}
            </a>
          ) : currentPackage?.downloadUrl ? (
            <a className="button secondary launch-link-button" href={currentPackage.downloadUrl}>
              {currentPackage.actionLabel || "Download current package"}
            </a>
          ) : null}
        </div>
        {canSuggestPasswords ? (
          <div className="setting-row">
            <div className="setting-row-copy">
              <strong>Suggest strong passwords on sign-up forms</strong>
              <span>
                When a page asks for a new password, the extension offers one generated with your Generator settings and saves the
                login for you. Applies to every browser you connect.
              </span>
            </div>
            <button
              type="button"
              role="switch"
              aria-checked={suggestPasswords === true}
              aria-label="Suggest strong passwords on sign-up forms"
              className={`switch ${suggestPasswords === true ? "on" : ""}`}
              disabled={suggestPasswords === null || busy}
              onClick={() => void toggleSuggestPasswords(suggestPasswords !== true)}
            >
              <span className="switch-knob" />
            </button>
          </div>
        ) : null}
        <div className="browser-extension-manager-grid">
          {visiblePackages.map((item) => (
            <section key={item.id} className="detail-section browser-extension-package-card">
              <div className="browser-extension-package-header">
                <div>
                  <p className="eyebrow">{item.packageType.toUpperCase()}</p>
                  <h3>{item.label}</h3>
                </div>
                <span className={`tag browser-extension-status browser-extension-status-${item.status}`}>
                  {browserExtensionStatusLabel(item.status)}
                </span>
              </div>
              <p className="detail-description">{item.notes}</p>
              {item.installUrl ? (
                <div className="action-row compact-actions">
                  <a className="button secondary launch-link-button" href={item.installUrl} target="_blank" rel="noreferrer">
                    {item.actionLabel || "Install add-on"}
                  </a>
                </div>
              ) : item.files.length > 0 ? (
                <ul className="artifact-file-list">
                  {item.files.map((file) => (
                    <li key={file.name} className="artifact-file">
                      <span className="artifact-file-info">
                        <span className="artifact-file-name">{file.name}</span>
                        {formatArtifactMeta(file) ? (
                          <span className="artifact-file-meta muted">{formatArtifactMeta(file)}</span>
                        ) : null}
                      </span>
                      <a className="button secondary launch-link-button artifact-download-button" href={file.downloadUrl}>
                        {item.actionLabel || `Download ${item.packageType.toUpperCase()}`}
                      </a>
                    </li>
                  ))}
                </ul>
              ) : (
                <span className="muted">Package not published yet</span>
              )}
            </section>
          ))}
        </div>
      </div>
    </div>
  );
}
