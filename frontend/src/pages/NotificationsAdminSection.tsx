import type { Dispatch, SetStateAction } from "react";
import type { AppRegistrationNotificationPolicy, NotificationAdminForm, NotificationDeliveryRecord } from "../types";

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
      <section className="panel">
        <div className="panel-header">
          <div>
            <p className="eyebrow">Default policy</p>
            <h2>App registration expiry reminders</h2>
          </div>
        </div>
        <p className="section-copy">
          Global defaults apply to every imported app registration unless a per-app or per-credential override replaces them.
        </p>
        <div className="form-grid">
          <label className="checkbox">
            <input
              type="checkbox"
              checked={form.appRegistrationNotificationPolicy.enabled}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  appRegistrationNotificationPolicy: {
                    ...current.appRegistrationNotificationPolicy,
                    enabled: event.target.checked
                  }
                }))
              }
            />
            <span>Enable expiry notifications</span>
          </label>
          <label>
            <span>Reminder days</span>
            <input
              value={form.appRegistrationNotificationPolicy.reminderDays.join(", ")}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  appRegistrationNotificationPolicy: {
                    ...current.appRegistrationNotificationPolicy,
                    reminderDays: event.target.value
                      .split(",")
                      .map((item) => Number(item.trim()))
                      .filter((item) => !Number.isNaN(item))
                  }
                }))
              }
            />
          </label>
          <label className="checkbox">
            <input
              type="checkbox"
              checked={form.appRegistrationNotificationPolicy.channels.includes("in_app")}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  appRegistrationNotificationPolicy: {
                    ...current.appRegistrationNotificationPolicy,
                    channels: event.target.checked
                      ? ([...new Set([...current.appRegistrationNotificationPolicy.channels, "in_app"])] as AppRegistrationNotificationPolicy["channels"])
                      : current.appRegistrationNotificationPolicy.channels.filter((item) => item !== "in_app")
                  }
                }))
              }
            />
            <span>In-app notification center</span>
          </label>
          <label className="checkbox">
            <input
              type="checkbox"
              checked={form.appRegistrationNotificationPolicy.channels.includes("email")}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  appRegistrationNotificationPolicy: {
                    ...current.appRegistrationNotificationPolicy,
                    channels: event.target.checked
                      ? ([...new Set([...current.appRegistrationNotificationPolicy.channels, "email"])] as AppRegistrationNotificationPolicy["channels"])
                      : current.appRegistrationNotificationPolicy.channels.filter((item) => item !== "email")
                  }
                }))
              }
            />
            <span>Email delivery</span>
          </label>
        </div>
        <div className="detail-grid compact-detail-grid">
          <div>
            <dt>Current days</dt>
            <dd>{summarizeReminderDays(form.appRegistrationNotificationPolicy.reminderDays)}</dd>
          </div>
          <div>
            <dt>Channels</dt>
            <dd>{form.appRegistrationNotificationPolicy.channels.join(", ") || "none"}</dd>
          </div>
        </div>
      </section>

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
