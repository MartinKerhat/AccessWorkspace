import { useState } from "react";

type Props = {
  // hasVault false = first-time setup (choose a passphrase); true = unlock.
  hasVault: boolean;
  busy: boolean;
  onSubmit: (passphrase: string) => Promise<boolean>;
  onCancel: () => void;
};

export function VaultUnlockModal({ hasVault, busy, onSubmit, onCancel }: Props) {
  const [passphrase, setPassphrase] = useState("");
  const [confirm, setConfirm] = useState("");

  const setup = !hasVault;
  const mismatch = setup && confirm !== "" && passphrase !== confirm;
  const tooShort = setup && passphrase !== "" && passphrase.length < 8;
  const ready = setup ? passphrase.length >= 8 && passphrase === confirm : passphrase !== "";

  return (
    <div className="modal-scrim" onClick={onCancel}>
      <div className="modal-card" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <p className="eyebrow">Personal vault</p>
            <h2>{setup ? "Set up your personal vault" : "Unlock your personal vault"}</h2>
          </div>
          <button className="button ghost" onClick={onCancel}>
            Close
          </button>
        </div>
        {setup ? (
          <p className="section-copy">
            Your personal saved passwords are encrypted with a passphrase only you know. Choose one now — it cannot be
            recovered if forgotten.
          </p>
        ) : (
          <p className="section-copy">Enter your vault passphrase to use your personal saved passwords this session.</p>
        )}
        <div className="form-grid">
          <label className="wide">
            <span>{setup ? "Vault passphrase" : "Passphrase"}</span>
            <input
              type="password"
              autoFocus
              value={passphrase}
              onChange={(event) => setPassphrase(event.target.value)}
              placeholder={setup ? "At least 8 characters" : ""}
              onKeyDown={(event) => {
                if (event.key === "Enter" && ready && !busy) {
                  void onSubmit(passphrase);
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
          <button className="button primary" disabled={busy || !ready} onClick={() => void onSubmit(passphrase)}>
            {busy ? "Working..." : setup ? "Set up vault" : "Unlock"}
          </button>
          <button className="button ghost" onClick={onCancel}>
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}
