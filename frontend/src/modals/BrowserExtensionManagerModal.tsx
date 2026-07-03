import type { BrowserExtensionPackage } from "../types";
import { formatArtifactMeta } from "../format";
import { browserExtensionStatusLabel } from "../browserExtension";

type Props = {
  visiblePackages: BrowserExtensionPackage[];
  currentPackage: BrowserExtensionPackage | null;
  busy: boolean;
  onClose: () => void;
  onConnect: () => void;
};

export function BrowserExtensionManagerModal({ visiblePackages, currentPackage, busy, onClose, onConnect }: Props) {
  return (
    <div className="modal-scrim" onClick={onClose}>
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
