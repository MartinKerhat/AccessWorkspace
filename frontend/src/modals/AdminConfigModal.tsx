import type { Dispatch, SetStateAction } from "react";
import type { AdminForm } from "../types";

type Props = {
  form: AdminForm;
  setForm: Dispatch<SetStateAction<AdminForm>>;
  clientSecretSet: boolean;
  busy: boolean;
  onSave: () => void;
  onClose: () => void;
};

export function AdminConfigModal({ form, setForm, clientSecretSet, busy, onSave, onClose }: Props) {
  return (
    <div className="modal-scrim" onClick={onClose}>
      <div className="modal-card" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <p className="eyebrow">Administration</p>
            <h2>Edit Azure / Entra connection</h2>
          </div>
          <button className="button ghost" onClick={onClose}>
            Close
          </button>
        </div>
        <div className="form-grid">
          <label className="wide checkbox">
            <input
              type="checkbox"
              checked={form.entraEnabled}
              onChange={(event) => setForm((current) => ({ ...current, entraEnabled: event.target.checked }))}
            />
            <span>Enable Microsoft sign-in on the login screen</span>
          </label>
          <label className="wide">
            <span>Authority</span>
            <input
              value={form.entraAuthority}
              onChange={(event) => setForm((current) => ({ ...current, entraAuthority: event.target.value }))}
              placeholder="https://login.microsoftonline.com"
            />
          </label>
          <label className="wide">
            <span>Redirect URI</span>
            <input
              value={form.entraRedirectUri}
              onChange={(event) => setForm((current) => ({ ...current, entraRedirectUri: event.target.value }))}
              placeholder="http://localhost:8080/api/auth/microsoft/callback"
            />
          </label>
          <label>
            <span>Tenant ID</span>
            <input
              value={form.entraTenantId}
              onChange={(event) => setForm((current) => ({ ...current, entraTenantId: event.target.value }))}
              placeholder="Azure tenant ID"
            />
          </label>
          <label>
            <span>Client ID</span>
            <input
              value={form.entraClientId}
              onChange={(event) => setForm((current) => ({ ...current, entraClientId: event.target.value }))}
              placeholder="App registration client ID"
            />
          </label>
          <label>
            <span>Group source</span>
            <select
              value={form.entraGroupSource}
              onChange={(event) => setForm((current) => ({ ...current, entraGroupSource: event.target.value }))}
            >
              <option value="graph">Microsoft Graph (group IDs)</option>
              <option value="token_claims">Token claims</option>
            </select>
          </label>
          <label className="wide">
            <span>Client secret</span>
            <input
              type="password"
              value={form.entraClientSecret}
              onChange={(event) => setForm((current) => ({ ...current, entraClientSecret: event.target.value }))}
              placeholder={clientSecretSet ? "Leave blank to keep stored secret" : "Client secret for confidential app flow"}
            />
          </label>
          <label className="wide checkbox">
            <input
              type="checkbox"
              checked={form.azureReaderUseAmbientIdentity}
              onChange={(event) =>
                setForm((current) => ({ ...current, azureReaderUseAmbientIdentity: event.target.checked }))
              }
            />
            <span>
              Key Vault and app registration reads use the ambient Azure identity (workload identity / managed
              identity) instead of the stored client secret. Microsoft sign-in keeps using the client secret. Enable
              only after the deployment identity is set up, otherwise sync and reveal will fail.
            </span>
          </label>
        </div>
        <div className="action-row">
          <button className="button primary" disabled={busy} onClick={onSave}>
            Save Entra settings
          </button>
          <button className="button ghost" onClick={onClose}>
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}
