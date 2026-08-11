import { useEffect, useRef, useState } from "react";
import type { BrowserExtensionRuntime, ConnectionCredentialOverride, DirectoryUser, LaunchPayload, LauncherRuntime, Resource, ResourceSummary } from "../types";
import { resourceTypeLabel, resourceTypeSummary } from "../resourceMeta";

function formatDateTime(value?: string) {
  if (!value) {
    return "n/a";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

function credentialState(value?: string) {
  if (!value) {
    return "no-expiry";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "unknown";
  }
  const now = Date.now();
  if (date.getTime() <= now) {
    return "expired";
  }
  if (date.getTime() <= now + 30 * 24 * 60 * 60 * 1000) {
    return "expiring";
  }
  return "active";
}

function credentialStateLabel(state: string) {
  return state === "no-expiry" ? "no expiry" : state;
}

type Props = {
  resource?: Resource;
  launch?: LaunchPayload | null;
  loading?: boolean;
  // True while a launch hand-off is in flight; only the launch buttons react
  // to it (the global loading flag intentionally stays out of the launch flow).
  launching?: boolean;
  canEdit?: boolean;
  canRemove?: boolean;
  // Owners (and admins on shared objects) may reveal password objects even
  // when the per-object reveal/copy policy is off — they control that policy.
  canOverrideRevealPolicy?: boolean;
  launcherRuntime?: LauncherRuntime | null;
  browserExtensionRuntime?: BrowserExtensionRuntime | null;
  passwordOptions?: ResourceSummary[];
  // Sharing directory used to resolve allowedUsers ids to display names.
  sharedUsers?: DirectoryUser[];
  connectionOverride?: ConnectionCredentialOverride | null;
  onEdit?: () => void;
  onEditNotifications?: () => void;
  onRemove?: () => Promise<void> | void;
  onPrepareBrowserExtension?: () => Promise<void>;
  onOpenBrowserExtensions?: () => void;
  onOpenLauncherDownloads?: () => void;
  onReveal: () => Promise<string | undefined>;
  onRevealOverridePassword?: () => Promise<string | undefined>;
  onRevealConnectionPassword?: () => Promise<string | undefined>;
  onLaunch: () => Promise<void>;
  onSaveConnectionOverride?: (passwordResourceId: string) => Promise<void>;
  onClearConnectionOverride?: () => Promise<void>;
};

function launchActionLabel(resource: Resource, launching?: boolean) {
  switch (resource.type) {
    case "rdp":
    case "ssh":
      return launching ? "Connecting…" : "Connect";
    case "web_portal":
      return launching ? "Opening…" : "Open target";
    default:
      return launching ? "Launching…" : "Launch";
  }
}

function showDetailLaunchAction(resource: Resource) {
  return resource.type === "rdp" || resource.type === "ssh" || (resource.type === "web_portal" && resource.launchAllowed);
}

function showDetailRevealAction(resource: Resource) {
  return resource.type === "key_vault_secret" && secretUsagePolicy(resource);
}

// Copy and reveal are one permission; the two fields are written together and
// either being set means the secret may be used. Mirrors the backend's
// secretUsageAllowed, including the OR that covers pre-unification rows.
function secretUsagePolicy(resource: Resource) {
  return resource.revealAllowed || resource.copyAllowed;
}

// Fetches a secret only when the user asks for it and caches it until resetKey
// changes, so the reveal is audited once per selection rather than once per
// click. resetKey should be a value whose identity changes whenever the
// underlying secret might have (the refetched resource, the override object).
function useLazySecret(reveal: (() => Promise<string | undefined>) | undefined, resetKey: unknown) {
  const [value, setValue] = useState("");
  const [visible, setVisible] = useState(false);
  const [message, setMessage] = useState("");

  useEffect(() => {
    setValue("");
    setVisible(false);
    setMessage("");
  }, [resetKey]);

  async function resolve() {
    if (value) {
      return value;
    }
    const next = await reveal?.();
    if (!next) {
      return "";
    }
    setValue(next);
    return next;
  }

  return {
    value,
    visible,
    message,
    async copy() {
      const next = await resolve();
      if (!next) {
        return;
      }
      try {
        await navigator.clipboard.writeText(next);
        setMessage("Password copied to clipboard");
      } catch {
        setMessage("Copying the password failed");
      }
    },
    async toggle() {
      if (visible) {
        setVisible(false);
        return;
      }
      if (!(await resolve())) {
        return;
      }
      setVisible(true);
    }
  };
}

// Same eye icon as the edit form's password field — one reveal affordance
// across the app, not a "Show" button here and an icon there.
function PasswordRevealRow({
  secret,
  disabled,
  ariaLabel
}: {
  secret: ReturnType<typeof useLazySecret>;
  disabled: boolean;
  ariaLabel: string;
}) {
  return (
    <div className="password-detail-row">
      <input
        className="password-detail-input"
        type={secret.visible ? "text" : "password"}
        value={secret.visible && secret.value ? secret.value : "••••••••••••"}
        readOnly
        aria-label={ariaLabel}
      />
      <button className="button ghost" disabled={disabled} onClick={() => void secret.copy()}>
        Copy password
      </button>
      <button
        type="button"
        className="password-visibility-button"
        disabled={disabled}
        onClick={() => void secret.toggle()}
        aria-label={secret.visible ? "Hide password" : "Show password"}
        title={secret.visible ? "Hide password" : "Show password"}
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
          {!secret.visible ? <path d="M4 20 20 4" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" /> : null}
        </svg>
      </button>
    </div>
  );
}

export function ResourceDetailPage({
  resource,
  launch,
  loading,
  launching,
  canEdit,
  canRemove,
  canOverrideRevealPolicy = false,
  launcherRuntime,
  browserExtensionRuntime,
  passwordOptions = [],
  sharedUsers = [],
  connectionOverride,
  onEdit,
  onEditNotifications,
  onRemove,
  onPrepareBrowserExtension,
  onOpenBrowserExtensions,
  onOpenLauncherDownloads,
  onReveal,
  onRevealOverridePassword,
  onRevealConnectionPassword,
  onLaunch,
  onSaveConnectionOverride,
  onClearConnectionOverride
}: Props) {
  const importedKeyVaultDescription =
    resource?.type === "key_vault_secret" && resource.description.startsWith("Imported from Azure Key Vault")
      ? ""
      : resource?.description ?? "";
  const isConnection = resource?.type === "ssh" || resource?.type === "rdp";
  const [selectedPasswordOptionId, setSelectedPasswordOptionId] = useState("");
  const [revealedPassword, setRevealedPassword] = useState("");
  const [passwordVisible, setPasswordVisible] = useState(false);
  const [passwordCopyMessage, setPasswordCopyMessage] = useState("");
  const [overrideExpanded, setOverrideExpanded] = useState(false);
  const [overridePickerOpen, setOverridePickerOpen] = useState(false);
  const [overrideSearch, setOverrideSearch] = useState("");
  const overrideSecret = useLazySecret(onRevealOverridePassword, connectionOverride);
  const connectionSecret = useLazySecret(onRevealConnectionPassword, resource);
  const overrideActive = Boolean(connectionOverride?.passwordResourceId);
  const overridePickerRef = useRef<HTMLDivElement | null>(null);

  // The picker is in-flow inside a scrolling panel, so opening it can leave the
  // options below the fold. Bring them into view instead of making the user hunt.
  useEffect(() => {
    if (overridePickerOpen) {
      overridePickerRef.current?.scrollIntoView({ block: "nearest" });
    }
  }, [overridePickerOpen]);

  const overrideOptions = passwordOptions.filter((item) => item.personal && item.type === "shared_secret");
  const filteredOverrideOptions = overrideOptions.filter((item) => {
    const query = overrideSearch.trim().toLowerCase();
    if (!query) {
      return true;
    }
    return `${item.name} ${item.username ?? ""}`.toLowerCase().includes(query);
  });
  const selectedOverrideOption = overrideOptions.find((item) => item.id === selectedPasswordOptionId);

  useEffect(() => {
    setSelectedPasswordOptionId(connectionOverride?.passwordResourceId ?? "");
  }, [connectionOverride?.passwordResourceId]);

  // The cached revealed password must not survive a refetch of the resource:
  // an edit-save keeps the same id but may change the stored password, and a
  // stale cache would make Reveal silently copy the pre-edit value. Keyed on
  // the object reference — every refetch produces a new object.
  useEffect(() => {
    setRevealedPassword("");
    setPasswordVisible(false);
    setPasswordCopyMessage("");
  }, [resource]);

  useEffect(() => {
    setOverrideExpanded(false);
    setOverridePickerOpen(false);
    setOverrideSearch("");
  }, [resource?.id]);

  if (!resource) {
    return (
      <section className="panel detail-panel empty-state">
        <p className="eyebrow">Details</p>
        <h2>Select a resource</h2>
        <p>Pick an item from the catalog to inspect access details and continue with the right operational action.</p>
      </section>
    );
  }

  const showLaunchAction = showDetailLaunchAction(resource);
  const showRevealAction = showDetailRevealAction(resource);

  async function handleCopyPassword() {
    if (!revealedPassword) {
      const secretValue = await onReveal();
      if (!secretValue) {
        return;
      }
      setRevealedPassword(secretValue);
      try {
        await navigator.clipboard.writeText(secretValue);
        setPasswordCopyMessage("Password copied to clipboard");
      } catch {
        setPasswordCopyMessage("Copying the password failed");
      }
      return;
    }
    try {
      await navigator.clipboard.writeText(revealedPassword);
      setPasswordCopyMessage("Password copied to clipboard");
    } catch {
      setPasswordCopyMessage("Copying the password failed");
    }
  }


  return (
    <section className="panel detail-panel">
      <div className="panel-header">
        <div className="detail-header-copy">
          <p className="eyebrow">Resource details</p>
          <h2>{resource.name}</h2>
          <p className="detail-description">{resourceTypeSummary(resource.type)}</p>
        </div>
        <div className="detail-header-actions">
          <span className={`resource-type ${resource.type}`}>{resourceTypeLabel(resource.type)}</span>
          {isConnection && showLaunchAction ? (
            <button className="button primary" disabled={loading || launching} onClick={() => void onLaunch()}>
              {launchActionLabel(resource, launching)}
            </button>
          ) : null}
          {isConnection && launcherRuntime?.downloadUrl ? (
            (launcherRuntime.downloads?.length ?? 0) > 1 && onOpenLauncherDownloads ? (
              <button className="button ghost" onClick={onOpenLauncherDownloads}>
                Download launcher
              </button>
            ) : (
              <a className="button ghost launch-link-button" href={launcherRuntime.downloadUrl}>
                Download launcher
              </a>
            )
          ) : null}
          {resource.type === "web_portal" && browserExtensionRuntime && onOpenBrowserExtensions ? (
            <button className="button ghost" onClick={onOpenBrowserExtensions}>
              Browser extensions
            </button>
          ) : resource.type === "web_portal" && browserExtensionRuntime?.downloadUrl ? (
            <a className="button ghost launch-link-button" href={browserExtensionRuntime.downloadUrl}>
              Download browser extension
            </a>
          ) : null}
          {resource.type === "web_portal" && onPrepareBrowserExtension && !onOpenBrowserExtensions ? (
            <button className="button ghost" onClick={() => void onPrepareBrowserExtension()}>
              Connect extension
            </button>
          ) : null}
          {canEdit && onEdit ? (
            <button className="button ghost" onClick={onEdit}>
              Edit
            </button>
          ) : null}
          {resource.type === "app_registration" && canEdit && onEditNotifications ? (
            <button className="button ghost" onClick={onEditNotifications}>
              Notifications
            </button>
          ) : null}
          {canRemove && onRemove ? (
            <button className="button ghost" onClick={() => void onRemove()}>
              Remove
            </button>
          ) : null}
        </div>
      </div>

      {importedKeyVaultDescription ? (
        <div className="detail-section">
          <p className="eyebrow">Purpose</p>
          <p className="detail-description">{importedKeyVaultDescription}</p>
        </div>
      ) : null}

      {resource.type === "key_vault_secret" && resource.status === "disabled" ? (
        <div className="detail-section">
          <p className="eyebrow">Azure state</p>
          <p className="detail-description">This Key Vault secret is currently disabled in Azure.</p>
        </div>
      ) : null}

      {resource.type === "app_registration" && (resource.status === "expiring" || resource.status === "expired" || resource.status === "no_credentials") ? (
        <div className="detail-section">
          <p className="eyebrow">Credential attention</p>
          <p className="detail-description">
            {resource.status === "expired"
              ? "One or more synced credentials have already expired."
              : resource.status === "no_credentials"
                ? "No client secrets or certificates are currently synced for this app registration."
                : "The next synced credential expires within 30 days."}
          </p>
        </div>
      ) : null}

      <dl className="detail-grid">
        <div>
          <dt>Owner</dt>
          <dd>{resource.owner}</dd>
        </div>
        {resource.type === "app_registration" ? (
          <div>
            <dt>Owner team</dt>
            <dd>{resource.ownerTeam || "n/a"}</dd>
          </div>
        ) : null}
        {resource.type !== "app_registration" ? (
          <div>
            <dt>Environment</dt>
            <dd>{resource.environment || "n/a"}</dd>
          </div>
        ) : null}
        <div>
          <dt>Status</dt>
          <dd>{resource.status || "n/a"}</dd>
        </div>
        <div>
          <dt>Shared with</dt>
          <dd>
            {resource.personal
              ? "Only you (personal)"
              : resource.allowedGroups.length > 0 || (resource.allowedUsers ?? []).length > 0
                ? [
                    ...resource.allowedGroups,
                    ...(resource.allowedUsers ?? []).map(
                      (userId) => sharedUsers.find((item) => item.id === userId)?.name ?? userId
                    )
                  ].join(", ")
                : "Everyone"}
          </dd>
        </div>
      </dl>

      {(resource.type === "ssh" || resource.type === "rdp") ? (
        <>
        <dl className="detail-grid">
          <div>
            <dt>Host</dt>
            <dd>{resource.targetHost || "n/a"}</dd>
          </div>
          <div>
            <dt>Port</dt>
            <dd>{resource.targetPort ?? "n/a"}</dd>
          </div>
          <div>
            <dt>Username</dt>
            <dd>{resource.username || "n/a"}</dd>
          </div>
          <div>
            <dt>Domain</dt>
            <dd>{resource.connectionDomain || "n/a"}</dd>
          </div>
        </dl>
        {onRevealConnectionPassword ? (
          <div className="detail-section">
            <p className="eyebrow">Connection password</p>
            <PasswordRevealRow secret={connectionSecret} disabled={Boolean(loading)} ariaLabel="Connection password" />
            <p className="detail-description">
              {resource.username
                ? `Sign in as ${resource.username}. Some servers enforce their own credential prompt and ignore the password the launcher passes — copy it here and paste it into that prompt.`
                : "Some servers enforce their own credential prompt and ignore the password the launcher passes — copy it here and paste it into that prompt."}
            </p>
            {connectionSecret.message ? <p className="detail-description">{connectionSecret.message}</p> : null}
          </div>
        ) : null}
        <div className="detail-section">
          <p className="eyebrow">Notes</p>
          <div className="connection-notes-card">
            <p className="connection-notes-copy">{resource.notes || "n/a"}</p>
          </div>
        </div>
        </>
      ) : null}

      {isConnection ? (
        <div className={`detail-section override-section${overrideActive ? " is-active" : ""}`}>
          <div className="override-summary-row">
            <div>
              <p className="eyebrow">My credential override</p>
              {overrideActive ? (
                <p className="override-name">{connectionOverride?.passwordResourceName || "Saved password"}</p>
              ) : (
                <p className="override-name is-default">Shared connection default</p>
              )}
            </div>
            <button className="button ghost" onClick={() => setOverrideExpanded((expanded) => !expanded)}>
              {overrideExpanded ? "Hide" : "Change"}
            </button>
          </div>
          {overrideActive && onRevealOverridePassword ? (
            <>
              <PasswordRevealRow secret={overrideSecret} disabled={Boolean(loading)} ariaLabel="Override password" />
              <p className="detail-description">
                {connectionOverride?.username
                  ? `The launcher uses this override instead of the connection's own password, signing in as ${connectionOverride.username}.`
                  : "The launcher uses this override instead of the connection's own password."}
              </p>
              {overrideSecret.message ? <p className="detail-description">{overrideSecret.message}</p> : null}
            </>
          ) : null}
          {overrideExpanded ? (
            <>
              <p className="detail-description">
                Keep the shared connection default, or pick one of your personal saved passwords to use your own username and
                password for this connection.
              </p>
              <div className="picker-shell picker-shell-inline" ref={overridePickerRef}>
                <button
                  type="button"
                  className="single-picker-trigger"
                  disabled={loading}
                  onClick={() => setOverridePickerOpen((open) => !open)}
                >
                  <span>
                    {selectedOverrideOption
                      ? `${selectedOverrideOption.name}${selectedOverrideOption.username ? ` (${selectedOverrideOption.username})` : ""}`
                      : "Use shared connection default"}
                  </span>
                  <span>{overridePickerOpen ? "Close" : "Select"}</span>
                </button>
                {overridePickerOpen ? (
                  <div className="group-picker-dropdown">
                    <input
                      className="picker-search-input"
                      value={overrideSearch}
                      onChange={(event) => setOverrideSearch(event.target.value)}
                      placeholder="Search your personal passwords"
                    />
                    <div className="picker-option-list">
                      <button
                        type="button"
                        className={`picker-option ${selectedPasswordOptionId === "" ? "active" : ""}`}
                        onClick={() => {
                          setSelectedPasswordOptionId("");
                          setOverridePickerOpen(false);
                          setOverrideSearch("");
                        }}
                      >
                        <strong>Use shared connection default</strong>
                        <span>No personal override for this connection</span>
                      </button>
                      {filteredOverrideOptions.map((item) => (
                        <button
                          key={item.id}
                          type="button"
                          className={`picker-option ${selectedPasswordOptionId === item.id ? "active" : ""}`}
                          onClick={() => {
                            setSelectedPasswordOptionId(item.id);
                            setOverridePickerOpen(false);
                            setOverrideSearch("");
                          }}
                        >
                          <strong>{item.name}</strong>
                          <span>{item.username || "no username"}</span>
                        </button>
                      ))}
                      {filteredOverrideOptions.length === 0 ? (
                        <span className="selection-hint">
                          {overrideOptions.length === 0
                            ? "You have no personal saved passwords yet. Create one under Passwords first."
                            : "No personal passwords match the current search."}
                        </span>
                      ) : null}
                    </div>
                  </div>
                ) : null}
              </div>
              <div className="action-row compact-actions">
                <button
                  className="button secondary"
                  disabled={loading || !selectedPasswordOptionId || selectedPasswordOptionId === connectionOverride?.passwordResourceId}
                  onClick={() => void onSaveConnectionOverride?.(selectedPasswordOptionId)}
                >
                  Save override
                </button>
                <button
                  className="button ghost"
                  disabled={loading || !connectionOverride?.passwordResourceId}
                  onClick={() => void onClearConnectionOverride?.()}
                >
                  Clear override
                </button>
              </div>
            </>
          ) : null}
        </div>
      ) : null}

      {resource.type === "web_portal" ? (
        <>
          <dl className="detail-grid">
            <div>
              <dt>Portal URL</dt>
              <dd>{resource.targetUrl || "n/a"}</dd>
            </div>
            <div>
              <dt>Username</dt>
              <dd>{resource.username || "n/a"}</dd>
            </div>
            <div>
              <dt>Password source</dt>
              <dd>
                {resource.secret.mode === "none"
                  ? "passwordless (SSO / emailed code)"
                  : resource.personal
                    ? "personal saved password"
                    : "shared saved password"}
              </dd>
            </div>
            <div>
              <dt>Copy / reveal / fill</dt>
              <dd>{resource.secret.mode === "none" ? "n/a" : secretUsagePolicy(resource) ? "allowed" : "disabled"}</dd>
            </div>
            <div>
              <dt>Open target policy</dt>
              <dd>{resource.launchAllowed ? "allowed" : "disabled"}</dd>
            </div>
          </dl>
          {resource.secret.mode === "none" ? (
            <div className="detail-section">
              <p className="eyebrow">Password</p>
              <p className="detail-description">
                This portal signs in without a stored password (SSO or a code sent by email). Check the notes for sign-in instructions.
              </p>
            </div>
          ) : (
            <div className="detail-section">
              <p className="eyebrow">Password</p>
              <div className="password-detail-row">
                <input
                  className="password-detail-input"
                  type={passwordVisible ? "text" : "password"}
                  value={revealedPassword || "••••••••••••"}
                  readOnly
                  aria-label="Stored password"
                />
                <button
                  className="button ghost"
                  disabled={loading || (!secretUsagePolicy(resource) && !canOverrideRevealPolicy)}
                  onClick={() => void handleCopyPassword()}
                >
                  Reveal
                </button>
              </div>
              {passwordCopyMessage ? <p className="detail-description">{passwordCopyMessage}</p> : null}
            </div>
          )}
        </>
      ) : null}

      {resource.type === "shared_secret" ? (
        <>
          <dl className="detail-grid">
            <div>
              <dt>Username</dt>
              <dd>{resource.username || "n/a"}</dd>
            </div>
            <div>
              <dt>Password source</dt>
              <dd>{resource.personal ? "personal saved password" : "shared saved password"}</dd>
            </div>
            <div>
              <dt>Stored system</dt>
              <dd>{resource.targetSystem || "n/a"}</dd>
            </div>
            <div>
              <dt>Copy / reveal</dt>
              <dd>{secretUsagePolicy(resource) ? "allowed" : "disabled"}</dd>
            </div>
          </dl>
          <div className="detail-section">
            <p className="eyebrow">Password</p>
            <div className="password-detail-row">
              <input
                className="password-detail-input"
                type={passwordVisible ? "text" : "password"}
                value={revealedPassword || "••••••••••••"}
                readOnly
                aria-label="Stored password"
              />
              <button
                className="button ghost"
                disabled={loading || (!secretUsagePolicy(resource) && !canOverrideRevealPolicy)}
                onClick={() => void handleCopyPassword()}
              >
                Reveal
              </button>
            </div>
            {passwordCopyMessage ? <p className="detail-description">{passwordCopyMessage}</p> : null}
          </div>
          <div className="detail-section">
            <p className="eyebrow">Notes</p>
            <div className="connection-notes-card">
              <p className="connection-notes-copy">{resource.notes || "n/a"}</p>
            </div>
          </div>
        </>
      ) : null}

      {resource.type === "key_vault_secret" ? (
        <dl className="detail-grid">
          <div>
            <dt>Vault</dt>
            <dd>{resource.vaultName || "n/a"}</dd>
          </div>
          <div>
            <dt>Object name</dt>
            <dd>{resource.objectName || "n/a"}</dd>
          </div>
          <div>
            <dt>Object type</dt>
            <dd>{resource.objectType || "n/a"}</dd>
          </div>
          <div>
            <dt>Version</dt>
            <dd>{resource.objectVersion || "latest"}</dd>
          </div>
          <div>
            <dt>Content type</dt>
            <dd>{resource.contentType || "n/a"}</dd>
          </div>
          <div>
            <dt>Expires at</dt>
            <dd>{formatDateTime(resource.expiresAt)}</dd>
          </div>
        </dl>
      ) : null}

      {resource.type === "app_registration" ? (
        <>
          <dl className="detail-grid">
            <div>
              <dt>Provider</dt>
              <dd>{resource.provider || "n/a"}</dd>
            </div>
            <div>
              <dt>Application ID</dt>
              <dd>{resource.applicationId || "n/a"}</dd>
            </div>
            <div>
              <dt>Tenant ID</dt>
              <dd>{resource.tenantId || "n/a"}</dd>
            </div>
            <div>
              <dt>Client ID</dt>
              <dd>{resource.clientId || "n/a"}</dd>
            </div>
            <div>
              <dt>Next credential type</dt>
              <dd>{resource.credentialType || "n/a"}</dd>
            </div>
            <div>
              <dt>Next credential expiry</dt>
              <dd>{formatDateTime(resource.credentialExpiresAt)}</dd>
            </div>
          </dl>
          <div className="detail-section">
            <p className="eyebrow">Synced credentials</p>
            {(resource.appCredentials ?? []).length > 0 ? (
              <div className="credential-list">
                {(resource.appCredentials ?? []).map((credential) => (
                  <div key={`${credential.credentialType}-${credential.keyId}`} className="credential-row">
                    <div>
                      <strong>{credential.displayName || credential.keyId}</strong>
                      <p>{credential.credentialType}</p>
                    </div>
                    <div>
                      <span className={`tag credential-state-${credentialState(credential.endDateTime)}`}>
                        {credentialStateLabel(credentialState(credential.endDateTime))}
                      </span>
                      <p>{formatDateTime(credential.endDateTime)}</p>
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <p className="detail-description">No client secrets or certificates are currently synced.</p>
            )}
          </div>
        </>
      ) : null}

      {resource.type !== "app_registration" && !isConnection && (showLaunchAction || showRevealAction) ? (
        <div className="action-row compact-actions">
          {showLaunchAction ? (
            <button className="button primary" disabled={loading || launching} onClick={() => void onLaunch()}>
              {launchActionLabel(resource, launching)}
            </button>
          ) : null}
          {showRevealAction ? (
            <button className="button secondary" disabled={loading} onClick={() => void onReveal()}>
              Reveal secret
            </button>
          ) : null}
        </div>
      ) : null}

      {launch && !isConnection ? (
          <div className="payload-box launch-box">
            <div className="launch-box-header">
              <div>
                <p className="eyebrow">Launch target</p>
                <p className="detail-description">
                  This target is ready to continue in the browser or through the current launch workflow.
                </p>
              </div>
              <span className="tag">{launch.method}</span>
            </div>
            <dl className="detail-grid">
              <div>
                <dt>Target</dt>
                <dd>{launch.target || resource.targetUrl || resource.targetHost || "n/a"}</dd>
              </div>
              <div>
                <dt>Secret mode</dt>
                <dd>{resource.secret.mode}</dd>
              </div>
            </dl>
            {launch.url ? (
              <div className="action-row compact-actions">
                <a className="button secondary launch-link-button" href={launch.url} target="_blank" rel="noreferrer">
                  Open target
                </a>
              </div>
            ) : null}
          </div>
        
      ) : null}

    </section>
  );
}
