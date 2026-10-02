import type { SessionInfo } from "../types";
import { relativeTime, sessionKindLabel } from "../sessions";
import { scrimDismissProps } from "./scrim";

type Props = {
  sessions: SessionInfo[];
  // Global status/error message mirrored inside the modal (the main banner
  // is hidden behind the scrim while this is open).
  message?: string;
  busy: boolean;
  onRevoke: (item: SessionInfo) => Promise<boolean>;
  onRevokeOthers: () => Promise<boolean>;
  onClose: () => void;
};

// Sessions & devices: every live session of the signed-in user (web and
// browser extension) with sign-out per row and sign-out-everywhere-else.
// Revocation is immediate server-side; a revoked session also loses its copy
// of the unlocked vault key.
export function SessionsModal({ sessions, message, busy, onRevoke, onRevokeOthers, onClose }: Props) {
  const others = sessions.filter((item) => !item.current).length;

  function confirmRevoke(item: SessionInfo) {
    if (item.current) {
      const confirmed = window.confirm(
        "Sign out this session?\n\nThis is the session you are using right now — you will be returned to the sign-in page."
      );
      if (!confirmed) {
        return;
      }
    }
    void onRevoke(item);
  }

  function confirmRevokeOthers() {
    const confirmed = window.confirm(
      `Sign out ${others} other ${others === 1 ? "session" : "sessions"}?\n\n` +
        "Every other browser and browser extension signed in as you will be signed out immediately. " +
        "This session stays."
    );
    if (confirmed) {
      void onRevokeOthers();
    }
  }

  return (
    <div className="modal-scrim" {...scrimDismissProps(onClose)}>
      <div className="modal-card sessions-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <p className="eyebrow">Account</p>
            <h2>Sessions &amp; devices</h2>
          </div>
          <button className="button ghost" onClick={onClose}>
            Close
          </button>
        </div>

        {message ? <div className="banner compact">{message}</div> : null}

        <p className="section-copy">
          Everywhere you are signed in right now. Signing a session out takes effect immediately, and that session can
          no longer open your personal passwords.
        </p>

        <div className="session-list">
          {sessions.map((item) => (
            <div key={item.id} className={`session-row ${item.current ? "current" : ""}`}>
              <div className="session-row-copy">
                <strong>
                  {item.client}
                  {item.current ? <span className="tag">this device</span> : null}
                </strong>
                <span>
                  {sessionKindLabel(item)}
                  {item.ip ? ` · ${item.ip}` : ""}
                  {item.vaultUnlocked ? " · vault unlocked" : ""}
                </span>
                <span>
                  Last active {relativeTime(item.lastUsedAt)} · signed in {new Date(item.createdAt).toLocaleString()} ·
                  expires {new Date(item.expiresAt).toLocaleString()}
                </span>
              </div>
              <div className="session-row-actions">
                <button className="button ghost" disabled={busy} onClick={() => confirmRevoke(item)}>
                  Sign out
                </button>
              </div>
            </div>
          ))}
        </div>

        {others > 0 ? (
          <div className="action-row">
            <button className="button ghost" disabled={busy} onClick={confirmRevokeOthers}>
              Sign out all other sessions
            </button>
          </div>
        ) : (
          <p className="section-copy">This is your only active session.</p>
        )}
      </div>
    </div>
  );
}
