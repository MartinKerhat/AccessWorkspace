import type { BrowserExtensionConnectState } from "../types";

type Props = {
  connectState: BrowserExtensionConnectState;
  hasRuntime: boolean;
  onClose: () => void;
  onOpenManager: () => void;
  onRetry: () => void;
};

export function BrowserExtensionConnectModal({ connectState, hasRuntime, onClose, onOpenManager, onRetry }: Props) {
  return (
    <div className="modal-scrim" onClick={onClose}>
      <div className="modal-card reveal-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <p className="eyebrow">Browser extension</p>
            <h2>Connect browser extension</h2>
          </div>
          <button className="button ghost" onClick={onClose}>
            Close
          </button>
        </div>
        <div className="detail-section">
          <p className="detail-description">
            {connectState.phase === "connecting"
              ? "Trying to connect the installed browser extension to this workspace."
              : connectState.phase === "connected"
                ? "The extension accepted the workspace handshake and now keeps its own long-lived session."
                : "The extension could not be reached from this page. Install or reload it, refresh this page, and try again."}
          </p>
          <p className="detail-description">Extension user: {connectState.user.name}</p>
          {connectState.error ? <p className="detail-description">{connectState.error}</p> : null}
        </div>
        {hasRuntime ? (
          <div className="action-row compact-actions">
            <button className="button secondary" onClick={onOpenManager}>
              Browser extensions
            </button>
            {connectState.phase === "unavailable" ? (
              <button className="button ghost" onClick={onRetry}>
                Try again
              </button>
            ) : null}
          </div>
        ) : null}
      </div>
    </div>
  );
}
