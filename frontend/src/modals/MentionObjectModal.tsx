import { useEffect, useState } from "react";
import type { MentionTarget } from "../types";


type Props = {
  target: MentionTarget;
  busy?: boolean;
  onReveal: (resourceId: string) => Promise<string | undefined>;
  onClose: () => void;
};

// Read-only quick view of an object mentioned in a note. Deliberately not an
// editor — editing stays in the owning module. The point is to answer "what is
// this credential" without leaving the page you are working on.
export function MentionObjectModal({ target, busy, onReveal, onClose }: Props) {
  const [secret, setSecret] = useState("");
  const [visible, setVisible] = useState(false);
  const [message, setMessage] = useState("");

  useEffect(() => {
    setSecret("");
    setVisible(false);
    setMessage("");
  }, [target.resourceId]);

  // Only saved passwords and no-access mentions reach this view. An accessible
  // Key Vault secret opens the Key Vault module's own reveal modal instead — it
  // has no username and no stored value, so this shape would misdescribe it.
  const denied = target.state !== "accessible";

  async function handleCopyUsername() {
    if (!target.username) {
      return;
    }
    try {
      await navigator.clipboard.writeText(target.username);
      setMessage("Username copied to clipboard");
    } catch {
      setMessage("Copying the username failed");
    }
  }

  async function resolveSecret() {
    if (secret) {
      return secret;
    }
    const value = await onReveal(target.resourceId);
    if (!value) {
      return "";
    }
    setSecret(value);
    return value;
  }

  async function handleCopy() {
    const value = await resolveSecret();
    if (!value) {
      return;
    }
    try {
      await navigator.clipboard.writeText(value);
      setMessage("Password copied to clipboard");
    } catch {
      setMessage("Copying the password failed");
    }
  }

  async function handleToggle() {
    if (visible) {
      setVisible(false);
      return;
    }
    if (!(await resolveSecret())) {
      return;
    }
    setVisible(true);
  }

  return (
    <div className="modal-scrim" onClick={onClose}>
      <div className="modal-card reveal-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <p className="eyebrow">{target.category === "keyvault" ? "Key Vault secret" : "Saved password"}</p>
            <h2>{target.name || "Object"}</h2>
          </div>
          <button className="button ghost" onClick={onClose}>
            Close
          </button>
        </div>

        {denied ? (
          <div className="detail-section">
            <p className="detail-description">
              You do not have access to this object. It is referenced here because it is relevant to this
              resource — ask its owner or an administrator if you need it.
            </p>
          </div>
        ) : (
          <>
            <div className="detail-section">
              <p className="eyebrow">Username</p>
              <div className="password-detail-row">
                <input
                  className="password-detail-input"
                  value={target.username || "n/a"}
                  readOnly
                  aria-label="Mentioned object username"
                />
                <button
                  className="button ghost"
                  disabled={busy || !target.username}
                  onClick={() => void handleCopyUsername()}
                >
                  Copy username
                </button>
                {/* Keeps this input exactly as wide as the password one below,
                    which shares its row with a second, icon-sized button. */}
                <span className="password-visibility-spacer" aria-hidden="true" />
              </div>
            </div>
            <div className="detail-section">
              <p className="eyebrow">Password</p>
              <div className="password-detail-row">
                <input
                  className="password-detail-input"
                  type={visible ? "text" : "password"}
                  value={visible && secret ? secret : "••••••••••••"}
                  readOnly
                  aria-label="Mentioned object password"
                />
                <button className="button ghost" disabled={busy} onClick={() => void handleCopy()}>
                  Copy password
                </button>
                <button
                  type="button"
                  className="password-visibility-button"
                  disabled={busy}
                  onClick={() => void handleToggle()}
                  aria-label={visible ? "Hide password" : "Show password"}
                  title={visible ? "Hide password" : "Show password"}
                >
                  <svg viewBox="0 0 24 24" aria-hidden="true">
                    <path
                      d="M2 12c2.4-4 5.8-6 10-6s7.6 2 10 6c-2.4 4-5.8 6-10 6s-7.6-2-10-6Z"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="1.8"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                    />
                    <circle cx="12" cy="12" r="3" fill="none" stroke="currentColor" strokeWidth="1.8" />
                    {!visible ? <path d="M4 20 20 4" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" /> : null}
                  </svg>
                </button>
              </div>
              {message ? <p className="detail-description">{message}</p> : null}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
