import type { Dispatch, SetStateAction } from "react";
import type { AdminConfig, AuthMode, NotificationAdminForm } from "../types";

type Props = {
  adminConfig: AdminConfig | null;
  sessionAuthMode: AuthMode;
  isAdmin: boolean;
  busy: boolean;
  keyVaultSyncing: boolean;
  notificationAdminForm: NotificationAdminForm;
  setNotificationAdminForm: Dispatch<SetStateAction<NotificationAdminForm>>;
  onEditEntra: () => void;
  onSyncKeyVault: () => void;
  onEditKeyVaultSources: () => void;
  onSaveAutoSync: () => void;
};

export function AzureAdminSection({
  adminConfig,
  sessionAuthMode,
  isAdmin,
  busy,
  keyVaultSyncing,
  notificationAdminForm,
  setNotificationAdminForm,
  onEditEntra,
  onSyncKeyVault,
  onEditKeyVaultSources,
  onSaveAutoSync
}: Props) {
  return (
    <div className="admin-grid">
      <section className="panel">
        <div className="panel-header">
          <div>
            <p className="eyebrow">Identity provider</p>
            <h2>Azure / Entra connection</h2>
          </div>
          <span className="muted">{adminConfig?.entraConfigured ? "Configured" : "Not configured"}</span>
        </div>
        <p className="section-copy">
          Set the tenant, application, and authority details used for Microsoft sign-in, Azure group resolution, Key Vault access, and app registration discovery.
        </p>
        <dl className="detail-grid">
          <div>
            <dt>SSO login</dt>
            <dd>{adminConfig?.entraEnabled ? "enabled" : "disabled"}</dd>
          </div>
          <div>
            <dt>Authority</dt>
            <dd>{adminConfig?.entraAuthority || "not set"}</dd>
          </div>
          <div>
            <dt>Tenant ID</dt>
            <dd>{adminConfig?.entraTenantId || "not set"}</dd>
          </div>
          <div>
            <dt>Client ID</dt>
            <dd>{adminConfig?.entraClientId || "not set"}</dd>
          </div>
          <div>
            <dt>Redirect URI</dt>
            <dd>{adminConfig?.entraRedirectUri || "not set"}</dd>
          </div>
          <div>
            <dt>Group source</dt>
            <dd>{adminConfig?.entraGroupSource || "not set"}</dd>
          </div>
          <div>
            <dt>Active auth mode</dt>
            <dd>{adminConfig?.authMode ?? sessionAuthMode}</dd>
          </div>
          <div>
            <dt>Client secret</dt>
            <dd>{adminConfig?.entraClientSecretSet ? "stored" : "not set"}</dd>
          </div>
          <div>
            <dt>Reader identity</dt>
            <dd>{adminConfig?.azureReaderUseAmbientIdentity ? "ambient (workload identity)" : "stored client secret"}</dd>
          </div>
          <div>
            <dt>Local groups</dt>
            <dd>{adminConfig?.localGroupCount ?? 0}</dd>
          </div>
          <div>
            <dt>Direct rights rules</dt>
            <dd>{adminConfig?.directRightsRuleCount ?? 0}</dd>
          </div>
        </dl>
        <div className="detail-section">
          <p className="eyebrow">App registration import</p>
          <p className="detail-description">
            Uses the stored Entra client credential flow, or the deployment's ambient Azure identity when the reader
            identity toggle is enabled. Either identity needs Microsoft Graph application permission
            Application.Read.All with admin consent for app registration import.
          </p>
        </div>
        <div className="action-row">
          <button className="button primary" onClick={onEditEntra}>
            Edit Entra connection
          </button>
        </div>
      </section>

      <section className="panel">
        <div className="panel-header">
          <div>
            <p className="eyebrow">Secret sources</p>
            <h2>Key Vault connections</h2>
          </div>
          <div className="panel-header-actions">
            <span className="muted">{adminConfig?.keyVaultSourceCount ?? 0} sources</span>
            {isAdmin ? (
              <button className="button ghost" disabled={busy || keyVaultSyncing} onClick={onSyncKeyVault}>
                {keyVaultSyncing ? "Syncing..." : "Sync now"}
              </button>
            ) : null}
          </div>
        </div>
        <p className="section-copy">
          Define which Azure Key Vault instances the workspace can browse and retrieve operational metadata from.
        </p>
        <div className="group-list">
          {(adminConfig?.keyVaultSources ?? []).length > 0 ? (
            adminConfig?.keyVaultSources.map((source) => (
              <article key={source.vaultUrl} className="group-card">
                <div className="group-card-section">
                  <strong>{source.name || source.vaultUrl}</strong>
                  <p>{source.vaultUrl}</p>
                </div>
                <div className="detail-grid compact-detail-grid">
                  <div>
                    <dt>Sync</dt>
                    <dd>{source.syncEnabled ? `every ${source.syncIntervalMinutes} min` : "disabled"}</dd>
                  </div>
                  <div>
                    <dt>Auto import</dt>
                    <dd>{source.autoImportEnabled ? "enabled" : "disabled"}</dd>
                  </div>
                  <div>
                    <dt>Default owner</dt>
                    <dd>{source.defaultOwner || "not set"}</dd>
                  </div>
                  <div>
                    <dt>Last synced</dt>
                    <dd>{source.lastSyncedAt ? new Date(source.lastSyncedAt).toLocaleString() : "never"}</dd>
                  </div>
                  <div>
                    <dt>Status</dt>
                    <dd>{source.lastSyncStatus || "not run"}</dd>
                  </div>
                  <div>
                    <dt>Summary</dt>
                    <dd>{source.lastSyncSummary || "No sync has been run for this source yet."}</dd>
                  </div>
                </div>
                {source.lastSyncError ? <p className="error-copy">{source.lastSyncError}</p> : null}
              </article>
            ))
          ) : (
            <article className="group-card">
              <p className="section-copy">No Key Vault source configured yet.</p>
            </article>
          )}
        </div>
        <div className="action-row">
          <button className="button primary" onClick={onEditKeyVaultSources}>
            Edit Key Vault sources
          </button>
        </div>
      </section>

      <section className="panel">
        <div className="panel-header">
          <div>
            <p className="eyebrow">Azure objects</p>
            <h2>Automatic app registration sync</h2>
          </div>
        </div>
        <p className="section-copy">
          This controls how often the backend re-checks imported Azure app registrations and evaluates expiry reminders.
        </p>
        <div className="form-grid">
          <label className="checkbox">
            <input
              type="checkbox"
              checked={notificationAdminForm.appRegistrationAutoSyncEnabled}
              onChange={(event) =>
                setNotificationAdminForm((current) => ({
                  ...current,
                  appRegistrationAutoSyncEnabled: event.target.checked
                }))
              }
            />
            <span>Enable automatic app registration sync</span>
          </label>
          <label>
            <span>Sync interval (minutes)</span>
            <input
              type="number"
              min={1}
              value={notificationAdminForm.appRegistrationSyncIntervalMinutes}
              onChange={(event) =>
                setNotificationAdminForm((current) => ({
                  ...current,
                  appRegistrationSyncIntervalMinutes: Number(event.target.value) || 60
                }))
              }
            />
          </label>
        </div>
        <div className="detail-grid compact-detail-grid">
          <div>
            <dt>Last synced</dt>
            <dd>{adminConfig?.appRegistrationLastSyncedAt ? new Date(adminConfig.appRegistrationLastSyncedAt).toLocaleString() : "never"}</dd>
          </div>
          <div>
            <dt>Status</dt>
            <dd>{adminConfig?.appRegistrationLastSyncStatus || "not run"}</dd>
          </div>
          <div className="wide">
            <dt>Summary</dt>
            <dd>{adminConfig?.appRegistrationLastSyncSummary || "No automatic or manual app registration sync has run yet."}</dd>
          </div>
        </div>
        {adminConfig?.appRegistrationLastSyncError ? <p className="error-copy">{adminConfig.appRegistrationLastSyncError}</p> : null}
        <div className="action-row">
          <button className="button primary" disabled={busy} onClick={onSaveAutoSync}>
            Save automatic sync settings
          </button>
        </div>
      </section>
    </div>
  );
}
