import { useState } from "react";
import type { VaultMethodDetail, VaultStatus } from "../types";
import { suggestPasskeyNickname } from "../hooks/useVault";

type Props = {
  status: VaultStatus;
  passkeyCapable: boolean;
  // Global status/error message mirrored inside the modal (the main banner
  // is hidden behind the scrim while this is open).
  message?: string;
  busy: boolean;
  onUnlock: () => void;
  onLock: () => Promise<void>;
  onAddPassphrase: (passphrase: string) => Promise<boolean>;
  onAddPasskey: (nickname: string) => Promise<boolean>;
  onRemoveMethod: (method: string, label: string) => Promise<boolean>;
  onRenamePasskey: (credentialId: string, nickname: string) => Promise<boolean>;
  onClose: () => void;
};

function methodDisplayName(detail: VaultMethodDetail): string {
  switch (detail.method) {
    case "password":
      return "Login password";
    case "passphrase":
      return "Passphrase";
    case "passkey":
      return detail.nickname || "Passkey";
    default:
      return detail.method;
  }
}

function methodDescription(detail: VaultMethodDetail): string {
  const added = new Date(detail.createdAt).toLocaleDateString();
  switch (detail.method) {
    case "password":
      return "Unlocks automatically when you sign in on this workspace.";
    case "passphrase":
      return `Works on any device. Added ${added}.`;
    case "passkey":
      return `Passkey — works on the device it was created on. Added ${added}.`;
    default:
      return `Added ${added}.`;
  }
}

// Vault settings: status + lock/unlock, and management of the unlock methods
// ("doors") — every method is an independent wrap of the same vault key, so
// any one of them opens all personal passwords. Management actions require an
// unlocked session (also enforced server-side).
export function VaultSettingsModal({
  status,
  passkeyCapable,
  message,
  busy,
  onUnlock,
  onLock,
  onAddPassphrase,
  onAddPasskey,
  onRemoveMethod,
  onRenamePasskey,
  onClose
}: Props) {
  const [passphraseFormOpen, setPassphraseFormOpen] = useState(false);
  const [passphrase, setPassphrase] = useState("");
  const [passphraseConfirm, setPassphraseConfirm] = useState("");
  const [passkeyFormOpen, setPasskeyFormOpen] = useState(false);
  const [passkeyNickname, setPasskeyNickname] = useState(suggestPasskeyNickname());
  const [renaming, setRenaming] = useState<{ label: string; value: string } | null>(null);

  const details = status.methodDetails ?? [];
  const hasPassphrase = details.some((detail) => detail.method === "passphrase");
  const removable = (detail: VaultMethodDetail) =>
    status.unlocked && detail.method !== "password" && details.length > 1;

  const passphraseMismatch = passphraseConfirm !== "" && passphrase !== passphraseConfirm;
  const passphraseTooShort = passphrase !== "" && passphrase.length < 8;
  const passphraseReady = passphrase.length >= 8 && passphrase === passphraseConfirm;

  async function submitPassphrase() {
    if (await onAddPassphrase(passphrase)) {
      setPassphrase("");
      setPassphraseConfirm("");
      setPassphraseFormOpen(false);
    }
  }

  async function submitPasskey() {
    if (await onAddPasskey(passkeyNickname)) {
      setPasskeyFormOpen(false);
      setPasskeyNickname(suggestPasskeyNickname());
    }
  }

  function confirmRemove(detail: VaultMethodDetail) {
    const name = methodDisplayName(detail);
    const confirmed = window.confirm(
      `Remove "${name}" as an unlock method?\n\n` +
        "A device or passphrase removed here can no longer open your personal passwords. " +
        "Your passwords stay intact — every remaining method still opens all of them."
    );
    if (confirmed) {
      void onRemoveMethod(detail.method, detail.label);
    }
  }

  return (
    <div className="modal-scrim" onClick={onClose}>
      <div className="modal-card vault-settings-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <p className="eyebrow">Personal passwords</p>
            <h2>Vault settings</h2>
          </div>
          <button className="button ghost" onClick={onClose}>
            Close
          </button>
        </div>

        {message ? <div className="banner compact">{message}</div> : null}

        {!status.hasVault ? (
          <>
            <p className="section-copy">
              Your personal passwords are not protected yet. Set up a passkey or passphrase to create your vault.
            </p>
            <div className="action-row">
              <button className="button primary" disabled={busy} onClick={onUnlock}>
                Set up now
              </button>
            </div>
          </>
        ) : (
          <>
            <div className="vault-status-row">
              <div>
                <strong>{status.unlocked ? "Unlocked" : "Locked"}</strong>
                <p className="section-copy">
                  {status.unlocked
                    ? "Personal passwords are available in this session."
                    : "Unlock to use personal passwords and manage the methods below."}
                </p>
              </div>
              {status.unlocked ? (
                <button className="button ghost" disabled={busy} onClick={() => void onLock()}>
                  Lock now
                </button>
              ) : (
                <button className="button primary" disabled={busy} onClick={onUnlock}>
                  Unlock
                </button>
              )}
            </div>

            {details.length === 1 ? (
              <div className="banner compact">
                You have only one unlock method. If you lose it, your personal passwords cannot be recovered. Add a
                backup — a passphrase works on any device.
              </div>
            ) : null}

            <div className="vault-method-list">
              {details.map((detail) => (
                <div key={`${detail.method}:${detail.label}`} className="vault-method-row">
                  <div className="vault-method-copy">
                    {renaming && detail.method === "passkey" && renaming.label === detail.label ? (
                      <div className="vault-method-rename">
                        <input
                          value={renaming.value}
                          autoFocus
                          maxLength={64}
                          onChange={(event) => setRenaming({ label: detail.label, value: event.target.value })}
                          onKeyDown={(event) => {
                            if (event.key === "Enter" && !busy) {
                              void onRenamePasskey(detail.label, renaming.value).then((ok) => {
                                if (ok) {
                                  setRenaming(null);
                                }
                              });
                            }
                            if (event.key === "Escape") {
                              setRenaming(null);
                            }
                          }}
                        />
                        <button
                          className="button ghost"
                          disabled={busy}
                          onClick={() =>
                            void onRenamePasskey(detail.label, renaming.value).then((ok) => {
                              if (ok) {
                                setRenaming(null);
                              }
                            })
                          }
                        >
                          Save
                        </button>
                        <button className="button ghost" onClick={() => setRenaming(null)}>
                          Cancel
                        </button>
                      </div>
                    ) : (
                      <>
                        <strong>{methodDisplayName(detail)}</strong>
                        <span>{methodDescription(detail)}</span>
                      </>
                    )}
                  </div>
                  <div className="vault-method-actions">
                    {detail.method === "passkey" && status.unlocked && !renaming ? (
                      <button
                        className="button ghost"
                        disabled={busy}
                        onClick={() => setRenaming({ label: detail.label, value: detail.nickname })}
                      >
                        Rename
                      </button>
                    ) : null}
                    {removable(detail) ? (
                      <button className="button ghost" disabled={busy} onClick={() => confirmRemove(detail)}>
                        Remove
                      </button>
                    ) : null}
                  </div>
                </div>
              ))}
            </div>

            {status.unlocked ? (
              <>
                <div className="action-row">
                  {!passphraseFormOpen ? (
                    <button
                      className="button ghost"
                      disabled={busy}
                      onClick={() => {
                        setPassphraseFormOpen(true);
                        setPasskeyFormOpen(false);
                      }}
                    >
                      {hasPassphrase ? "Change passphrase" : "Add passphrase"}
                    </button>
                  ) : null}
                  {passkeyCapable && !passkeyFormOpen ? (
                    <button
                      className="button ghost"
                      disabled={busy}
                      onClick={() => {
                        setPasskeyFormOpen(true);
                        setPassphraseFormOpen(false);
                      }}
                    >
                      Add a passkey for this device
                    </button>
                  ) : null}
                </div>

                {passphraseFormOpen ? (
                  <div className="vault-method-form">
                    <div className="form-grid">
                      <label>
                        <span>{hasPassphrase ? "New passphrase" : "Passphrase"}</span>
                        <input
                          type="password"
                          value={passphrase}
                          placeholder="At least 8 characters"
                          onChange={(event) => setPassphrase(event.target.value)}
                        />
                      </label>
                      <label>
                        <span>Confirm passphrase</span>
                        <input
                          type="password"
                          value={passphraseConfirm}
                          onChange={(event) => setPassphraseConfirm(event.target.value)}
                          onKeyDown={(event) => {
                            if (event.key === "Enter" && passphraseReady && !busy) {
                              void submitPassphrase();
                            }
                          }}
                        />
                      </label>
                    </div>
                    {passphraseTooShort ? <p className="section-copy">Passphrase must be at least 8 characters.</p> : null}
                    {passphraseMismatch ? <p className="section-copy">Passphrases do not match.</p> : null}
                    <div className="action-row">
                      <button className="button primary" disabled={busy || !passphraseReady} onClick={() => void submitPassphrase()}>
                        {busy ? "Saving..." : "Save passphrase"}
                      </button>
                      <button className="button ghost" onClick={() => setPassphraseFormOpen(false)}>
                        Cancel
                      </button>
                    </div>
                  </div>
                ) : null}

                {passkeyFormOpen ? (
                  <div className="vault-method-form">
                    <div className="form-grid">
                      <label className="wide">
                        <span>Name this device</span>
                        <input
                          value={passkeyNickname}
                          maxLength={64}
                          placeholder="Work laptop"
                          onChange={(event) => setPasskeyNickname(event.target.value)}
                        />
                      </label>
                    </div>
                    <p className="section-copy">
                      Your device will ask for its fingerprint, face, or PIN (Windows Hello, Touch ID) to create the passkey.
                    </p>
                    <div className="action-row">
                      <button className="button primary" disabled={busy} onClick={() => void submitPasskey()}>
                        {busy ? "Waiting for your passkey..." : "Create passkey"}
                      </button>
                      <button className="button ghost" onClick={() => setPasskeyFormOpen(false)}>
                        Cancel
                      </button>
                    </div>
                  </div>
                ) : null}
              </>
            ) : (
              <p className="section-copy">Unlock the vault to add, rename, or remove unlock methods.</p>
            )}
          </>
        )}
      </div>
    </div>
  );
}
