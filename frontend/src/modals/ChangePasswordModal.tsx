import { useState } from "react";

type Props = {
  busy: boolean;
  onSave: (currentPassword: string, newPassword: string) => Promise<boolean>;
  onClose: () => void;
};

export function ChangePasswordModal({ busy, onSave, onClose }: Props) {
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");

  const mismatch = confirmPassword !== "" && newPassword !== confirmPassword;
  const tooShort = newPassword !== "" && newPassword.length < 8;
  const ready = currentPassword !== "" && newPassword.length >= 8 && newPassword === confirmPassword;

  return (
    <div className="modal-scrim" onClick={onClose}>
      <div className="modal-card" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <p className="eyebrow">Account</p>
            <h2>Change password</h2>
          </div>
          <button className="button ghost" onClick={onClose}>
            Close
          </button>
        </div>
        <p className="section-copy">Your personal saved passwords stay intact through a password change.</p>
        <div className="form-grid">
          <label className="wide">
            <span>Current password</span>
            <input type="password" value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} />
          </label>
          <label className="wide">
            <span>New password</span>
            <input
              type="password"
              value={newPassword}
              onChange={(event) => setNewPassword(event.target.value)}
              placeholder="At least 8 characters"
            />
          </label>
          <label className="wide">
            <span>Confirm new password</span>
            <input type="password" value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} />
          </label>
        </div>
        {tooShort ? <p className="section-copy">New password must be at least 8 characters.</p> : null}
        {mismatch ? <p className="section-copy">New passwords do not match.</p> : null}
        <div className="action-row">
          <button
            className="button primary"
            disabled={busy || !ready}
            onClick={() => {
              void onSave(currentPassword, newPassword).then((saved) => {
                if (saved) {
                  onClose();
                }
              });
            }}
          >
            {busy ? "Saving..." : "Change password"}
          </button>
          <button className="button ghost" onClick={onClose}>
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}
