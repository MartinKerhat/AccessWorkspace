import { useState } from "react";

type Props = {
  // hasVault false = first-time setup; true = unlock an existing vault.
  hasVault: boolean;
  // passkeyCapable: this browser has a platform authenticator (Hello/Touch ID).
  passkeyCapable: boolean;
  // hasPasskey: the vault has at least one registered passkey (unlock only).
  hasPasskey: boolean;
  busy: boolean;
  onPasskey: () => Promise<boolean>;
  onPassphrase: (passphrase: string) => Promise<boolean>;
  onCancel: () => void;
};

export function VaultUnlockModal({ hasVault, passkeyCapable, hasPasskey, busy, onPasskey, onPassphrase, onCancel }: Props) {
  const setup = !hasVault;
  // Hello is offered for setup whenever the device supports it, and for
  // unlock only when a passkey is actually registered.
  const offerPasskey = passkeyCapable && (setup || hasPasskey);
  // Passphrase is the fallback; default-visible only when Hello isn't offered.
  const [showPassphrase, setShowPassphrase] = useState(!offerPasskey);
  const [passphrase, setPassphrase] = useState("");
  const [confirm, setConfirm] = useState("");

  const mismatch = setup && confirm !== "" && passphrase !== confirm;
  const tooShort = setup && passphrase !== "" && passphrase.length < 8;
  const ready = setup ? passphrase.length >= 8 && passphrase === confirm : passphrase !== "";

  return (
    <div className="modal-scrim" onClick={onCancel}>
      <div className="modal-card" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <p className="eyebrow">Personal passwords</p>
            <h2>{setup ? "Protect your personal passwords" : "Unlock your personal passwords"}</h2>
          </div>
          <button className="button ghost" onClick={onCancel}>
            Close
          </button>
        </div>

        {setup ? (
          <p className="section-copy">
            {offerPasskey
              ? "Use a passkey — your device's fingerprint, face, or PIN (Windows Hello, Touch ID, or a security key) — so only you can open your personal saved passwords. Nothing to remember."
              : "Choose a passphrase to encrypt your personal saved passwords. Only you know it, and it cannot be recovered if forgotten."}
          </p>
        ) : (
          <p className="section-copy">Confirm it's you to use your personal saved passwords this session.</p>
        )}

        {offerPasskey ? (
          <div className="action-row">
            <button className="button primary" disabled={busy} onClick={() => void onPasskey()}>
              {busy ? "Waiting for your passkey..." : setup ? "Set up a passkey" : "Unlock with a passkey"}
            </button>
          </div>
        ) : null}

        {offerPasskey && !showPassphrase ? (
          <button className="button ghost" onClick={() => setShowPassphrase(true)}>
            Use a passphrase instead
          </button>
        ) : null}

        {showPassphrase ? (
          <>
            {offerPasskey ? <div className="login-divider"><span>or a passphrase</span></div> : null}
            <div className="form-grid">
              <label className="wide">
                <span>{setup ? "New passphrase" : "Passphrase"}</span>
                <input
                  type="password"
                  value={passphrase}
                  onChange={(event) => setPassphrase(event.target.value)}
                  placeholder={setup ? "At least 8 characters" : ""}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" && ready && !busy) {
                      void onPassphrase(passphrase);
                    }
                  }}
                />
              </label>
              {setup ? (
                <label className="wide">
                  <span>Confirm passphrase</span>
                  <input type="password" value={confirm} onChange={(event) => setConfirm(event.target.value)} />
                </label>
              ) : null}
            </div>
            {tooShort ? <p className="section-copy">Passphrase must be at least 8 characters.</p> : null}
            {mismatch ? <p className="section-copy">Passphrases do not match.</p> : null}
            <div className="action-row">
              <button className="button primary" disabled={busy || !ready} onClick={() => void onPassphrase(passphrase)}>
                {busy ? "Working..." : setup ? "Save passphrase" : "Unlock"}
              </button>
              <button className="button ghost" onClick={onCancel}>
                Cancel
              </button>
            </div>
          </>
        ) : null}
      </div>
    </div>
  );
}
