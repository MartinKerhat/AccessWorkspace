import type { Dispatch, SetStateAction } from "react";
import type { NotificationPolicyModalState } from "../types";

type ResourceState = Extract<NotificationPolicyModalState, { mode: "resource" }>;

type Props = {
  state: ResourceState;
  setState: Dispatch<SetStateAction<NotificationPolicyModalState>>;
  busy: boolean;
  onSave: () => void;
  onClose: () => void;
};

export function NotificationPolicyModal({ state, setState, busy, onSave, onClose }: Props) {
  function toggleChannel(channel: "in_app" | "email") {
    setState((current) => {
      if (current.mode !== "resource") {
        return current;
      }
      const channels = current.draft.channels.includes(channel)
        ? current.draft.channels.filter((item) => item !== channel)
        : [...current.draft.channels, channel];
      return { ...current, draft: { ...current.draft, channels } };
    });
  }

  return (
    <div className="modal-scrim" onClick={onClose}>
      <div className="modal-card" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <p className="eyebrow">Notification policy</p>
            <h2>{state.resource.name}</h2>
          </div>
          <button className="button ghost" onClick={onClose}>
            Close
          </button>
        </div>
        <div className="form-grid">
          <label className="checkbox">
            <input
              type="checkbox"
              checked={state.useResourceOverride}
              onChange={(event) =>
                setState((current) =>
                  current.mode !== "resource" ? current : { ...current, useResourceOverride: event.target.checked }
                )
              }
            />
            <span>Override inherited policy for this app</span>
          </label>
          <label className="checkbox">
            <input
              type="checkbox"
              checked={state.draft.enabled}
              onChange={(event) =>
                setState((current) =>
                  current.mode !== "resource"
                    ? current
                    : { ...current, draft: { ...current.draft, enabled: event.target.checked } }
                )
              }
            />
            <span>Enable notifications for this app</span>
          </label>
          <label>
            <span>Reminder days</span>
            <input
              value={state.draft.reminderDays.join(", ")}
              disabled={!state.useResourceOverride}
              onChange={(event) =>
                setState((current) =>
                  current.mode !== "resource"
                    ? current
                    : {
                        ...current,
                        draft: {
                          ...current.draft,
                          reminderDays: event.target.value
                            .split(",")
                            .map((item) => Number(item.trim()))
                            .filter((item) => !Number.isNaN(item))
                        }
                      }
                )
              }
            />
          </label>
          <label className="checkbox">
            <input
              type="checkbox"
              checked={state.draft.channels.includes("in_app")}
              disabled={!state.useResourceOverride}
              onChange={() => toggleChannel("in_app")}
            />
            <span>In-app notification center</span>
          </label>
          <label className="checkbox">
            <input
              type="checkbox"
              checked={state.draft.channels.includes("email")}
              disabled={!state.useResourceOverride}
              onChange={() => toggleChannel("email")}
            />
            <span>Email delivery</span>
          </label>
        </div>
        <div className="detail-section">
          <p className="eyebrow">Credential overrides</p>
          <div className="credential-list">
            {(state.resource.appCredentials ?? []).map((credential) => {
              const draft = state.credentialDrafts.find((item) => item.keyId === credential.keyId)?.policy;
              return (
                <div key={credential.keyId} className="credential-row">
                  <div>
                    <strong>{credential.displayName || credential.keyId}</strong>
                    <p>{credential.credentialType}</p>
                  </div>
                  <div className="detail-section">
                    <label className="checkbox">
                      <input
                        type="checkbox"
                        checked={Boolean(draft)}
                        onChange={(event) =>
                          setState((current) => {
                            if (current.mode !== "resource") {
                              return current;
                            }
                            const next = current.credentialDrafts.filter((item) => item.keyId !== credential.keyId);
                            if (event.target.checked) {
                              next.push({
                                keyId: credential.keyId,
                                policy: credential.notificationPolicyOverride ?? current.draft
                              });
                            }
                            return { ...current, credentialDrafts: next };
                          })
                        }
                      />
                      <span>Override</span>
                    </label>
                    {draft ? (
                      <input
                        value={draft.reminderDays.join(", ")}
                        onChange={(event) =>
                          setState((current) => {
                            if (current.mode !== "resource") {
                              return current;
                            }
                            return {
                              ...current,
                              credentialDrafts: current.credentialDrafts.map((item) =>
                                item.keyId === credential.keyId
                                  ? {
                                      ...item,
                                      policy: {
                                        ...(item.policy ?? current.draft),
                                        reminderDays: event.target.value
                                          .split(",")
                                          .map((value) => Number(value.trim()))
                                          .filter((value) => !Number.isNaN(value))
                                      }
                                    }
                                  : item
                              )
                            };
                          })
                        }
                      />
                    ) : null}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
        <div className="action-row">
          <button className="button primary" disabled={busy} onClick={onSave}>
            Save policy
          </button>
        </div>
      </div>
    </div>
  );
}
