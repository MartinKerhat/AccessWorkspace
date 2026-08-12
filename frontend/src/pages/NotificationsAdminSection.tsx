import type { Dispatch, SetStateAction } from "react";
import type {
  ExpiryNotificationPolicy,
  NotificationAdminForm,
  NotificationChannel,
  NotificationDeliveryRecord
} from "../types";

type Props = {
  form: NotificationAdminForm;
  setForm: Dispatch<SetStateAction<NotificationAdminForm>>;
  emailConfigured: boolean;
  emailPasswordSet: boolean;
  busy: boolean;
  deliveries: NotificationDeliveryRecord[];
  onSaveSettings: () => void;
  onRefreshLog: () => void;
};

function summarizeReminderDays(days: number[]) {
  return days.length > 0 ? days.join(", ") : "none";
}

function toggleChannel(
  policy: ExpiryNotificationPolicy,
  channel: NotificationChannel,
  enabled: boolean
): ExpiryNotificationPolicy {
  return {
    ...policy,
    channels: enabled
      ? ([...new Set([...policy.channels, channel])] as NotificationChannel[])
      : policy.channels.filter((item) => item !== channel)
  };
}

type PolicyEditorProps = {
  eyebrow: string;
  heading: string;
  copy: string;
  policy: ExpiryNotificationPolicy;
  onChange: (next: ExpiryNotificationPolicy) => void;
};

// One editor, used for both expiry sources. App registrations and Key Vault
// secrets keep separate stored policies (their cadences differ) but the fields
// are identical, so the markup is shared rather than copied per category.
function ExpiryPolicyEditor({ eyebrow, heading, copy, policy, onChange }: PolicyEditorProps) {
  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <p className="eyebrow">{eyebrow}</p>
          <h2>{heading}</h2>
        </div>
      </div>
      <p className="section-copy">{copy}</p>
      <div className="form-grid">
        <label className="checkbox">
          <input
            type="checkbox"
            checked={policy.enabled}
            onChange={(event) => onChange({ ...policy, enabled: event.target.checked })}
          />
          <span>Enable expiry notifications</span>
        </label>
        <label>
          <span>Reminder days</span>
          <input
            value={policy.reminderDays.join(", ")}
            onChange={(event) =>
              onChange({
                ...policy,
                reminderDays: event.target.value
                  .split(",")
                  .map((item) => Number(item.trim()))
                  .filter((item) => !Number.isNaN(item))
              })
            }
          />
        </label>
        <label className="checkbox">
          <input
            type="checkbox"
            checked={policy.channels.includes("in_app")}
            onChange={(event) => onChange(toggleChannel(policy, "in_app", event.target.checked))}
          />
          <span>In-app notification center</span>
        </label>
        <label className="checkbox">
          <input
            type="checkbox"
            checked={policy.channels.includes("email")}
            onChange={(event) => onChange(toggleChannel(policy, "email", event.target.checked))}
          />
          <span>Email delivery</span>
        </label>
      </div>
      <div className="detail-grid compact-detail-grid">
        <div>
          <dt>Current days</dt>
          <dd>{summarizeReminderDays(policy.reminderDays)}</dd>
        </div>
        <div>
          <dt>Channels</dt>
          <dd>{policy.channels.join(", ") || "none"}</dd>
        </div>
      </div>
    </section>
  );
}

export function NotificationsAdminSection({
  form,
  setForm,
  emailConfigured,
  emailPasswordSet,
  busy,
  deliveries,
  onSaveSettings,
  onRefreshLog
}: Props) {
  return (
    <div className="admin-grid">
      <ExpiryPolicyEditor
        eyebrow="Default policy"
        heading="App registration expiry reminders"
        copy="Global defaults apply to every imported app registration unless a per-app or per-credential override replaces them."
        policy={form.appRegistrationNotificationPolicy}
        onChange={(next) => setForm((current) => ({ ...current, appRegistrationNotificationPolicy: next }))}
      />

      <ExpiryPolicyEditor
        eyebrow="Default policy"
        heading="Key Vault expiry reminders"
        copy="Applies to imported Key Vault secrets that carry an expiry date. Secrets without one never generate a reminder."
        policy={form.keyVaultNotificationPolicy}
        onChange={(next) => setForm((current) => ({ ...current, keyVaultNotificationPolicy: next }))}
      />

      <section className="panel">
        <div className="panel-header">
          <div>
            <p className="eyebrow">Email sender</p>
            <h2>SMTP delivery</h2>
          </div>
          <span className="muted">{emailConfigured ? "Configured" : "Not configured"}</span>
        </div>
        <div className="form-grid">
          <label className="checkbox">
            <input
              type="checkbox"
              checked={form.notificationEmailEnabled}
              onChange={(event) => setForm((current) => ({ ...current, notificationEmailEnabled: event.target.checked }))}
            />
            <span>Enable outbound email</span>
          </label>
          <label>
            <span>SMTP host</span>
            <input
              value={form.notificationEmailHost}
              onChange={(event) => setForm((current) => ({ ...current, notificationEmailHost: event.target.value }))}
            />
          </label>
          <label>
            <span>SMTP port</span>
            <input
              type="number"
              value={form.notificationEmailPort}
              onChange={(event) => setForm((current) => ({ ...current, notificationEmailPort: Number(event.target.value) || 587 }))}
            />
          </label>
          <label>
            <span>Username</span>
            <input
              value={form.notificationEmailUsername}
              onChange={(event) => setForm((current) => ({ ...current, notificationEmailUsername: event.target.value }))}
            />
          </label>
          <label>
            <span>Password</span>
            <input
              type="password"
              placeholder={emailPasswordSet ? "Leave blank to keep stored password" : ""}
              value={form.notificationEmailPassword}
              onChange={(event) => setForm((current) => ({ ...current, notificationEmailPassword: event.target.value }))}
            />
          </label>
          <label>
            <span>From address</span>
            <input
              value={form.notificationEmailFrom}
              onChange={(event) => setForm((current) => ({ ...current, notificationEmailFrom: event.target.value }))}
            />
          </label>
        </div>
        <div className="action-row">
          <button className="button primary" disabled={busy} onClick={onSaveSettings}>
            Save notification settings
          </button>
        </div>
      </section>

      <section className="panel admin-grid-span-two">
        <div className="panel-header">
          <div>
            <p className="eyebrow">Delivery log</p>
            <h2>Recent email reminders</h2>
          </div>
          <button className="button ghost" disabled={busy} onClick={onRefreshLog}>
            Refresh log
          </button>
        </div>
        <p className="section-copy">
          Recent app registration reminder emails, including failed delivery attempts, so we can diagnose SMTP issues without opening backend logs.
        </p>
        {deliveries.length === 0 ? (
          <p className="section-copy">No app registration reminder emails have been attempted yet.</p>
        ) : (
          <div className="notification-delivery-list">
            {deliveries.map((item) => (
              <div key={item.id} className="notification-delivery-item">
                <div className="notification-delivery-head">
                  <div>
                    <strong>{item.title}</strong>
                    <p>
                      {item.userName}
                      {item.userEmail ? ` (${item.userEmail})` : ""}
                    </p>
                  </div>
                  <span className={`tag ${item.emailStatus === "failed" ? "delivery-status-failed" : item.emailStatus === "sent" ? "delivery-status-sent" : ""}`}>
                    {item.emailStatus || "pending"}
                  </span>
                </div>
                <div className="detail-grid compact-detail-grid">
                  <div>
                    <dt>Resource</dt>
                    <dd>{item.resourceName}</dd>
                  </div>
                  <div>
                    <dt>Credential</dt>
                    <dd>{item.credentialType} {item.credentialDisplayName}</dd>
                  </div>
                  <div>
                    <dt>Reminder day</dt>
                    <dd>{item.reminderDay}</dd>
                  </div>
                  <div>
                    <dt>Created</dt>
                    <dd>{new Date(item.createdAt).toLocaleString()}</dd>
                  </div>
                  <div>
                    <dt>Sent</dt>
                    <dd>{item.emailSentAt ? new Date(item.emailSentAt).toLocaleString() : "not sent"}</dd>
                  </div>
                </div>
                {item.emailError ? <p className="error-copy">{item.emailError}</p> : null}
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
