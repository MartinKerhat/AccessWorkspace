import { useEffect, useState } from "react";
import type { DirectoryUser, Resource, ResourceForm, UserSummary } from "../types";
import { MentionNotesField } from "./MentionNotesField";

const defaultForm: ResourceForm = {
  name: "",
  type: "ssh",
  personal: false,
  description: "",
  owner: "",
  ownerUserId: "",
  ownerTeam: "",
  environment: "",
  status: "active",
  folderPath: "",
  launchMode: "native_launcher",
  sourceKind: "manual",
  sourceObjectId: "",
  notes: "",
  targetHost: "",
  targetUrl: "",
  targetSystem: "",
  username: "",
  connectionDomain: "",
  connectionAdminSession: false,
  connectionAutomaticLogon: false,
  connectionWindowMode: "launcher_default",
  connectionUseMultipleMonitors: false,
  connectionShowConnectionBar: true,
  connectionScreenMode: "launcher_default",
  connectionMacAddress: "",
  connectionGatewayHost: "",
  vaultName: "",
  objectName: "",
  objectType: "",
  objectVersion: "",
  contentType: "",
  provider: "",
  applicationId: "",
  tenantId: "",
  clientId: "",
  credentialType: "",
  displayNameExternal: "",
  linkedSecretRef: "",
  launchAllowed: true,
  // Sharing an object means the people it is shared with can use it, which
  // includes reading and copying its secret. Defaults only — the creator can
  // turn either off, and existing objects keep whatever they were saved with.
  revealAllowed: true,
  copyAllowed: true,
  allowedGroups: [],
  allowedUsers: [],
  secretMode: "inline",
  secretValue: "",
  secretReference: ""
};

function toDateTimeInputValue(value?: string) {
  if (!value) {
    return "";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  const year = date.getFullYear();
  const month = `${date.getMonth() + 1}`.padStart(2, "0");
  const day = `${date.getDate()}`.padStart(2, "0");
  const hours = `${date.getHours()}`.padStart(2, "0");
  const minutes = `${date.getMinutes()}`.padStart(2, "0");
  return `${year}-${month}-${day}T${hours}:${minutes}`;
}

function fromDateTimeInputValue(value: string) {
  if (!value) {
    return undefined;
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return undefined;
  }
  return date.toISOString();
}

// One field for every stored secret in this form. It used to be duplicated per
// object category, which is how connections ended up without the reveal icon
// and with different placeholder behaviour from passwords. autoComplete is
// "new-password" so the browser's own password manager does not autofill a
// credential field and get it saved over the real secret on submit.
function SecretField({
  label,
  value,
  placeholder,
  disabled,
  visible,
  revealAvailable,
  revealLoading,
  wide,
  onChange,
  onToggleVisibility
}: {
  label: string;
  value: string;
  placeholder: string;
  disabled: boolean;
  visible: boolean;
  revealAvailable: boolean;
  revealLoading: boolean;
  wide?: boolean;
  onChange: (value: string) => void;
  onToggleVisibility: () => void;
}) {
  return (
    <label className={wide ? "wide" : undefined}>
      <span>{label}</span>
      <div className={revealAvailable ? "password-field-row" : undefined}>
        <input
          disabled={disabled}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          type={visible ? "text" : "password"}
          placeholder={placeholder}
          autoComplete="new-password"
          spellCheck={false}
        />
        {revealAvailable ? (
          <button
            type="button"
            className="password-visibility-button"
            disabled={revealLoading}
            onClick={onToggleVisibility}
            aria-label={visible ? "Hide stored password" : "Show stored password"}
            title={visible ? "Hide stored password" : "Show stored password"}
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
              {!visible ? <path d="M4 20 20 4" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" /> : null}
            </svg>
          </button>
        ) : null}
      </div>
    </label>
  );
}

type Props = {
  resource?: Resource;
  initialType?: ResourceForm["type"];
  availableGroups?: string[];
  // Sharing directory for the visibility picker (every user, not admin-only).
  availableUsers?: DirectoryUser[];
  availableOwners?: UserSummary[];
  // New password objects start as personal so nobody shares a secret by
  // accident; saving as shared asks for confirmation instead of being blocked.
  defaultPersonalPassword?: boolean;
  // Only admins may assign a real owner; everyone else owns what they create.
  canAssignOwner?: boolean;
  sharedMetadataOnly?: boolean;
  loading?: boolean;
  onSubmit: (input: ResourceForm) => Promise<void>;
  onArchive?: () => Promise<void>;
  onRevealStoredPassword?: () => Promise<string | undefined>;
};

type PickerOption<T extends string> = {
  value: T;
  label: string;
  description?: string;
};

function categoryForType(type: ResourceForm["type"]): "connections" | "passwords" | "keyvault" | "appregistrations" {
  switch (type) {
    case "ssh":
    case "rdp":
      return "connections";
    case "web_portal":
    case "shared_secret":
      return "passwords";
    case "key_vault_secret":
      return "keyvault";
    case "app_registration":
      return "appregistrations";
  }
}

export function ResourceFormCard({
  resource,
  initialType,
  availableGroups = [],
  availableUsers = [],
  availableOwners = [],
  defaultPersonalPassword = false,
  canAssignOwner = false,
  sharedMetadataOnly = false,
  loading,
  onSubmit,
  onArchive,
  onRevealStoredPassword
}: Props) {
  // Non-owners editing a shared object may change only descriptive metadata
  // (description, notes, folder path, environment); everything else is locked.
  // The backend enforces the same rule, this is just honest UI.
  const coreLocked = sharedMetadataOnly;
  const [form, setForm] = useState<ResourceForm>(defaultForm);
  const [passwordVisible, setPasswordVisible] = useState(false);
  const [passwordRevealLoading, setPasswordRevealLoading] = useState(false);
  const [groupPickerOpen, setGroupPickerOpen] = useState(false);
  const [typePickerOpen, setTypePickerOpen] = useState(false);
  const [ownerPickerOpen, setOwnerPickerOpen] = useState(false);
  const [ownerTeamPickerOpen, setOwnerTeamPickerOpen] = useState(false);
  const [statusPickerOpen, setStatusPickerOpen] = useState(false);
  const [sourceKindPickerOpen, setSourceKindPickerOpen] = useState(false);
  const [secretModePickerOpen, setSecretModePickerOpen] = useState(false);
  const [ownerSearch, setOwnerSearch] = useState("");
  const [ownerTeamSearch, setOwnerTeamSearch] = useState("");
  const [sharedSearch, setSharedSearch] = useState("");

  useEffect(() => {
    if (!resource) {
      const nextType = initialType ?? defaultForm.type;
      setForm({
        ...defaultForm,
        type: nextType,
        personal: defaultPersonalPassword && categoryForType(nextType) === "passwords"
      });
      setPasswordVisible(false);
      return;
    }
    setPasswordVisible(false);
    setForm({
      name: resource.name,
      type: resource.type,
      personal: resource.personal,
      description: resource.description,
      owner: resource.owner,
      ownerUserId: resource.ownerUserId,
      ownerTeam: resource.ownerTeam,
      environment: resource.environment,
      status: resource.status,
      folderPath: resource.folderPath,
      launchMode: resource.launchMode,
      sourceKind: resource.sourceKind,
      sourceObjectId: resource.sourceObjectId,
      lastSyncedAt: resource.lastSyncedAt,
      notes: resource.notes,
      targetHost: resource.targetHost,
      targetPort: resource.targetPort,
      targetUrl: resource.targetUrl,
      targetSystem: resource.targetSystem,
      username: resource.username,
      connectionDomain: resource.connectionDomain,
      connectionAdminSession: resource.connectionAdminSession,
      connectionAutomaticLogon: resource.connectionAutomaticLogon,
      connectionWindowMode: resource.connectionWindowMode,
      connectionUseMultipleMonitors: resource.connectionUseMultipleMonitors,
      connectionShowConnectionBar: resource.connectionShowConnectionBar,
      connectionScreenMode: resource.connectionScreenMode,
      connectionMacAddress: resource.connectionMacAddress,
      connectionGatewayHost: resource.connectionGatewayHost,
      vaultName: resource.vaultName,
      objectName: resource.objectName,
      objectType: resource.objectType,
      objectVersion: resource.objectVersion,
      contentType: resource.contentType,
      expiresAt: resource.expiresAt,
      provider: resource.provider,
      applicationId: resource.applicationId,
      tenantId: resource.tenantId,
      clientId: resource.clientId,
      credentialType: resource.credentialType,
      credentialExpiresAt: resource.credentialExpiresAt,
      displayNameExternal: resource.displayNameExternal,
      linkedSecretRef: resource.linkedSecretRef,
      launchAllowed: resource.launchAllowed,
      revealAllowed: resource.revealAllowed,
      copyAllowed: resource.copyAllowed,
      allowedGroups: resource.allowedGroups,
      allowedUsers: resource.allowedUsers ?? [],
      secretMode: resource.secret.mode,
      secretValue: "",
      secretReference: resource.secret.reference
    });
  }, [resource, initialType]);

  function update<K extends keyof ResourceForm>(key: K, value: ResourceForm[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  // Copy and reveal are one permission — anyone who can put a secret on the
  // clipboard can read it, so splitting them protected nothing. The two fields
  // stay in the API for compatibility and are always written together.
  const secretUsageAllowed = form.revealAllowed || form.copyAllowed;

  function setSecretUsageAllowed(allowed: boolean) {
    setForm((current) => ({ ...current, revealAllowed: allowed, copyAllowed: allowed }));
  }

  function toggleAllowedGroup(group: string) {
    update(
      "allowedGroups",
      form.allowedGroups.includes(group)
        ? form.allowedGroups.filter((item) => item !== group)
        : [...form.allowedGroups, group]
    );
  }

  function toggleAllowedUser(userId: string) {
    update(
      "allowedUsers",
      form.allowedUsers.includes(userId)
        ? form.allowedUsers.filter((item) => item !== userId)
        : [...form.allowedUsers, userId]
    );
  }

  function allowedUserLabel(userId: string) {
    return availableUsers.find((item) => item.id === userId)?.name ?? userId;
  }

  async function handlePasswordVisibilityToggle() {
    if (passwordVisible) {
      setPasswordVisible(false);
      return;
    }
    if (!form.secretValue && resource && onRevealStoredPassword) {
      setPasswordRevealLoading(true);
      try {
        const revealed = await onRevealStoredPassword();
        if (revealed) {
          update("secretValue", revealed);
        }
      } finally {
        setPasswordRevealLoading(false);
      }
    }
    setPasswordVisible(true);
  }

  function closeOtherPickers(...keepOpen: string[]) {
    if (!keepOpen.includes("group")) {
      setGroupPickerOpen(false);
    }
    if (!keepOpen.includes("type")) {
      setTypePickerOpen(false);
    }
    if (!keepOpen.includes("owner")) {
      setOwnerPickerOpen(false);
    }
    if (!keepOpen.includes("ownerTeam")) {
      setOwnerTeamPickerOpen(false);
    }
    if (!keepOpen.includes("status")) {
      setStatusPickerOpen(false);
    }
    if (!keepOpen.includes("sourceKind")) {
      setSourceKindPickerOpen(false);
    }
    if (!keepOpen.includes("secretMode")) {
      setSecretModePickerOpen(false);
    }
  }

  const category = form.type === "ssh" || form.type === "rdp"
    ? "Connections"
    : form.type === "key_vault_secret"
      ? "Key Vault"
      : form.type === "app_registration"
        ? "App registrations"
        : "Passwords";
  const resourceCategory = categoryForType(resource?.type ?? initialType ?? form.type);
  const isConnectionResource = form.type === "ssh" || form.type === "rdp";
  // The secret value is never sent to the browser, so the field is always blank
  // on edit. This flag lets us show that a password IS stored (leave blank to
  // keep it) instead of the field looking empty/unset.
  // An external-reference secret has no inline value, so hasValue is false for
  // it — but a secret IS configured and blank still keeps it. Without the
  // reference clause the hint appeared on some objects and not others.
  const storedSecretPresent = Boolean(resource?.secret?.hasValue) || Boolean(resource?.secret?.reference);
  const storedSecretPlaceholder = storedSecretPresent ? "•••••••••• — stored, leave blank to keep" : "";
  const isPasswordResource = form.type === "web_portal" || form.type === "shared_secret";
  const isSharedPassword = form.type === "shared_secret";
  const isWebPortalPassword = form.type === "web_portal";
  // Portals that sign in without a stored password (SSO, emailed code,
  // passkey) keep URL + username but have no secret to store or reveal.
  const isPasswordlessPortal = isWebPortalPassword && form.secretMode === "none";
  const isImportedKeyVault =
    Boolean(resource) &&
    form.type === "key_vault_secret" &&
    form.sourceKind === "azure_key_vault";
  const isImportedAppRegistration =
    Boolean(resource) &&
    form.type === "app_registration" &&
    form.sourceKind === "entra_app_registration";
  const isManagedExternalSource = isImportedKeyVault || isImportedAppRegistration;
  const filteredOwners = availableOwners.filter((owner) => {
    const query = ownerSearch.trim().toLowerCase();
    if (query === "") {
      return true;
    }
    return owner.name.toLowerCase().includes(query) || owner.email.toLowerCase().includes(query);
  });
  const filteredOwnerTeams = availableGroups.filter((group) => group.toLowerCase().includes(ownerTeamSearch.trim().toLowerCase()));
  // One search box filters both sharing sections. Filtering is strict — the
  // current selection always stays visible as tags on the trigger above.
  const sharedSearchQuery = sharedSearch.trim().toLowerCase();
  const filteredSharedGroups = availableGroups.filter(
    (group) => sharedSearchQuery === "" || group.toLowerCase().includes(sharedSearchQuery)
  );
  const filteredSharedUsers = availableUsers.filter(
    (item) =>
      sharedSearchQuery === "" ||
      item.name.toLowerCase().includes(sharedSearchQuery) ||
      item.email.toLowerCase().includes(sharedSearchQuery)
  );
  const allTypeOptions: Array<{ value: ResourceForm["type"]; label: string }> = [
    { value: "ssh", label: "SSH" },
    { value: "rdp", label: "RDP" },
    { value: "web_portal", label: "Web portal login" },
    { value: "shared_secret", label: "Saved password" },
    { value: "key_vault_secret", label: "Key Vault ref" },
    { value: "app_registration", label: "App registration" }
  ];
  const typeOptions = allTypeOptions.filter((option) => categoryForType(option.value) === resourceCategory);
  const statusOptions: Array<PickerOption<string>> = [
    { value: "active", label: "active" },
    { value: "inactive", label: "inactive" }
  ];
  const sourceKindOptions: Array<PickerOption<ResourceForm["sourceKind"]>> = [
    { value: "manual", label: "Manual" },
    { value: "azure_key_vault", label: "Azure Key Vault" },
    { value: "entra_app_registration", label: "Entra app registration" }
  ];
  const secretModeOptions: Array<PickerOption<ResourceForm["secretMode"]>> = [
    { value: "inline", label: "Stored encrypted" },
    { value: "external_reference", label: "External reference" },
    ...((form.type === "ssh" || form.type === "rdp")
      ? [{ value: "prompt_on_launch" as const, label: "Prompt on launch" }]
      : [])
  ];

  function renderPicker<T extends string>({
    open,
    value,
    placeholder,
    options,
    onToggle,
    onSelect,
    disabled = false
  }: {
    open: boolean;
    value: T | string;
    placeholder: string;
    options: Array<PickerOption<T>>;
    onToggle: () => void;
    onSelect: (next: T) => void;
    disabled?: boolean;
  }) {
    const selected = options.find((option) => option.value === value);
    return (
      <div className="picker-shell">
        <button
          type="button"
          className="single-picker-trigger"
          disabled={disabled}
          onClick={onToggle}
        >
          <span>{selected?.label ?? (value || placeholder)}</span>
          <span>{disabled ? "" : open ? "Close" : "Select"}</span>
        </button>
        {open && !disabled ? (
          <div className="group-picker-dropdown">
            <div className="picker-option-list">
              {options.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  className={`picker-option ${value === option.value ? "active" : ""}`}
                  onClick={() => {
                    onSelect(option.value);
                    closeOtherPickers();
                  }}
                >
                  <strong>{option.label}</strong>
                  {option.description ? <span>{option.description}</span> : null}
                </button>
              ))}
            </div>
          </div>
        ) : null}
      </div>
    );
  }

  return (
    <section className="panel form-panel">
      <div className="panel-header">
        <div>
          <p className="eyebrow">Admin</p>
          <h2>{resource ? "Edit object" : "Create object"}</h2>
          <p className="section-copy">{category}</p>
        </div>
        {resource && onArchive ? (
          <button type="button" className="button ghost" onClick={() => void onArchive()}>
            Remove from app
          </button>
        ) : null}
      </div>

      <div className="form-grid">
        {isPasswordResource ? (
          <label className="checkbox wide">
            <input
              type="checkbox"
              checked={form.personal}
              disabled={coreLocked}
              onChange={(event) => update("personal", event.target.checked)}
            />
            <span>Personal saved password</span>
          </label>
        ) : null}
        <label>
          <span>Name</span>
          <input disabled={isImportedAppRegistration || coreLocked} value={form.name} onChange={(event) => update("name", event.target.value)} />
        </label>
        <label>
          <span>Type</span>
          {renderPicker({
            open: typePickerOpen,
            value: form.type,
            placeholder: "Select type",
            options: typeOptions,
            disabled: isManagedExternalSource || coreLocked,
            onToggle: () => {
              closeOtherPickers("type");
              setTypePickerOpen((open) => !open);
            },
            onSelect: (value) =>
              setForm((current) => ({
                ...current,
                type: value,
                personal: (value === "web_portal" || value === "shared_secret") ? current.personal : false,
                sourceKind: (value === "web_portal" || value === "shared_secret") ? "manual" : current.sourceKind,
                sourceObjectId: (value === "web_portal" || value === "shared_secret") ? "" : current.sourceObjectId,
                secretMode: (value === "web_portal" || value === "shared_secret") ? "inline" : current.secretMode,
                secretReference: (value === "web_portal" || value === "shared_secret") ? "" : current.secretReference
              }))
          })}
        </label>
        <label>
          <span>Owner</span>
          <div className="picker-shell">
            <button
              type="button"
              className="single-picker-trigger"
              disabled={form.personal || coreLocked || !canAssignOwner}
              onClick={() => {
                closeOtherPickers("owner");
                setOwnerPickerOpen((open) => !open);
              }}
            >
              <span>
                {form.owner ||
                  (form.personal
                    ? "Saved under current user"
                    : canAssignOwner
                      ? "Select owner"
                      : "You (creator)")}
              </span>
              <span>{ownerPickerOpen ? "Close" : "Select"}</span>
            </button>
            {ownerPickerOpen ? (
              <div className="group-picker-dropdown">
                <input
                  className="picker-search-input"
                  value={ownerSearch}
                  onChange={(event) => setOwnerSearch(event.target.value)}
                  placeholder="Search users"
                />
                <div className="picker-option-list">
                  {filteredOwners.map((owner) => (
                    <button
                      key={owner.id}
                      type="button"
                      className={`picker-option ${form.ownerUserId === owner.id ? "active" : ""}`}
                      onClick={() => {
                        setForm((current) => ({ ...current, owner: owner.name, ownerUserId: owner.id }));
                        setOwnerPickerOpen(false);
                        setOwnerSearch("");
                      }}
                    >
                      <strong>{owner.name}</strong>
                      <span>{owner.email}</span>
                    </button>
                  ))}
                  {filteredOwners.length === 0 ? <span className="selection-hint">No users match the current search.</span> : null}
                </div>
              </div>
            ) : null}
          </div>
        </label>
        {!isPasswordResource ? (
          <label>
            <span>Owner team</span>
            <div className="picker-shell">
              <button
                type="button"
                className="single-picker-trigger"
                disabled={form.personal || coreLocked}
                onClick={() => {
                  closeOtherPickers("ownerTeam");
                  setOwnerTeamPickerOpen((open) => !open);
                }}
              >
                <span>{form.ownerTeam || (form.personal ? "Not used for personal passwords" : "Select owner team")}</span>
                <span>{ownerTeamPickerOpen ? "Close" : "Select"}</span>
              </button>
              {ownerTeamPickerOpen ? (
                <div className="group-picker-dropdown">
                  <input
                    className="picker-search-input"
                    value={ownerTeamSearch}
                    onChange={(event) => setOwnerTeamSearch(event.target.value)}
                    placeholder="Search local groups"
                  />
                  <div className="picker-option-list">
                    {filteredOwnerTeams.map((group) => (
                      <button
                        key={group}
                        type="button"
                        className={`picker-option ${form.ownerTeam === group ? "active" : ""}`}
                        onClick={() => {
                          update("ownerTeam", group);
                          setOwnerTeamPickerOpen(false);
                          setOwnerTeamSearch("");
                        }}
                      >
                        <strong>{group}</strong>
                      </button>
                    ))}
                    {filteredOwnerTeams.length === 0 ? <span className="selection-hint">No groups match the current search.</span> : null}
                  </div>
                </div>
              ) : null}
            </div>
          </label>
        ) : null}
        {!isPasswordResource ? (
          <>
            <label>
              <span>Environment</span>
              <input value={form.environment} onChange={(event) => update("environment", event.target.value)} />
            </label>
            <label>
              <span>Folder path</span>
              <input value={form.folderPath} onChange={(event) => update("folderPath", event.target.value)} placeholder="Workspace/Infra" />
            </label>
            <label>
              <span>Status</span>
              {renderPicker({
                open: statusPickerOpen,
                value: form.status,
                placeholder: "Select status",
                options: statusOptions,
                disabled: isImportedAppRegistration || coreLocked,
                onToggle: () => {
                  closeOtherPickers("status");
                  setStatusPickerOpen((open) => !open);
                },
                onSelect: (value) => update("status", value)
              })}
            </label>
          </>
        ) : null}
        <label className="wide">
          <span>Description</span>
          <textarea value={form.description} onChange={(event) => update("description", event.target.value)} rows={3} />
        </label>
        <label className="wide">
          <span>Notes</span>
          {/* NOT gated on coreLocked: notes are one of the four fields a
              non-owner editor of a shared object is explicitly allowed to
              change (see the banner above and restrictToSharedMetadata). */}
          <MentionNotesField value={form.notes} rows={3} onChange={(next) => update("notes", next)} />
        </label>
        <label className="wide">
          <span>Shared with</span>
          {form.personal ? (
            <div className="selection-grid">
              <span className="selection-hint">Personal password objects are visible only to their creator.</span>
            </div>
          ) : availableGroups.length > 0 || availableUsers.length > 0 ? (
            <div className="picker-shell">
              <button
                type="button"
                className="group-picker-trigger"
                disabled={coreLocked}
                onClick={() => {
                  closeOtherPickers("group");
                  setGroupPickerOpen((open) => !open);
                }}
              >
                <div className="group-picker-value">
                  {form.allowedGroups.length > 0 || form.allowedUsers.length > 0 ? (
                    <>
                      {form.allowedGroups.map((group) => (
                        <span key={`group-${group}`} className="tag">
                          {group}
                        </span>
                      ))}
                      {form.allowedUsers.map((userId) => (
                        <span key={`user-${userId}`} className="tag">
                          {allowedUserLabel(userId)}
                        </span>
                      ))}
                    </>
                  ) : (
                    <span className="selection-hint">Everyone</span>
                  )}
                </div>
                <span>{groupPickerOpen ? "Close" : "Select"}</span>
              </button>
              {groupPickerOpen ? (
                <div className="group-picker-dropdown">
                  <input
                    className="picker-search-input"
                    value={sharedSearch}
                    onChange={(event) => setSharedSearch(event.target.value)}
                    placeholder="Search groups and users"
                  />
                  {filteredSharedGroups.length > 0 ? (
                    <>
                      <span className="selection-hint">Local groups</span>
                      {filteredSharedGroups.map((group) => (
                        <label key={group} className="checkbox">
                          <input
                            type="checkbox"
                            checked={form.allowedGroups.includes(group)}
                            onChange={() => toggleAllowedGroup(group)}
                          />
                          <span>{group}</span>
                        </label>
                      ))}
                    </>
                  ) : null}
                  {filteredSharedUsers.length > 0 ? (
                    <>
                      <span className="selection-hint">Specific users</span>
                      {filteredSharedUsers.map((item) => (
                        <label key={item.id} className="checkbox">
                          <input
                            type="checkbox"
                            checked={form.allowedUsers.includes(item.id)}
                            onChange={() => toggleAllowedUser(item.id)}
                          />
                          <span>
                            {item.name} <span className="selection-hint">{item.email}</span>
                          </span>
                        </label>
                      ))}
                    </>
                  ) : null}
                  {filteredSharedGroups.length === 0 && filteredSharedUsers.length === 0 ? (
                    <span className="selection-hint">No groups or users match the current search.</span>
                  ) : null}
                </div>
              ) : null}
            </div>
          ) : (
            <div className="selection-grid">
              <span className="selection-hint">No local groups or users available. Everyone can see this resource.</span>
            </div>
          )}
        </label>
        {!isPasswordResource ? (
          <>
            <label>
              <span>Source kind</span>
              {renderPicker({
                open: sourceKindPickerOpen,
                value: form.sourceKind,
                placeholder: "Select source kind",
                options: sourceKindOptions,
                disabled: isManagedExternalSource || coreLocked,
                onToggle: () => {
                  closeOtherPickers("sourceKind");
                  setSourceKindPickerOpen((open) => !open);
                },
                onSelect: (value) => update("sourceKind", value)
              })}
            </label>
            <label>
              <span>Source object ID</span>
              <input disabled={isManagedExternalSource || coreLocked} value={form.sourceObjectId} onChange={(event) => update("sourceObjectId", event.target.value)} />
            </label>
            <label className="checkbox">
              <input
                type="checkbox"
                disabled={isImportedAppRegistration || coreLocked}
                checked={form.launchAllowed}
                onChange={(event) => update("launchAllowed", event.target.checked)}
              />
              <span>Launch allowed</span>
            </label>
            {/* Connections need this too: a target that forces its own
                credential prompt ignores the password the launcher passes, so
                whoever the connection is shared with has to be able to read it
                and type it. Owners and admins can always reveal regardless. */}
            <label className="checkbox wide">
              <input
                type="checkbox"
                disabled={isImportedAppRegistration || coreLocked}
                checked={secretUsageAllowed}
                onChange={(event) => setSecretUsageAllowed(event.target.checked)}
              />
              <span>Copy and reveal allowed</span>
            </label>
          </>
        ) : null}
        {isPasswordResource ? (
          <>
            {isWebPortalPassword ? (
              <label className="checkbox">
                <input
                  type="checkbox"
                  disabled={coreLocked}
                  checked={form.launchAllowed}
                  onChange={(event) => update("launchAllowed", event.target.checked)}
                />
                <span>Open target allowed</span>
              </label>
            ) : null}
            {!isPasswordlessPortal ? (
              <label className={`checkbox${isWebPortalPassword ? "" : " wide"}`}>
                <input
                  type="checkbox"
                  disabled={coreLocked}
                  checked={secretUsageAllowed}
                  onChange={(event) => setSecretUsageAllowed(event.target.checked)}
                />
                <span>{isWebPortalPassword ? "Copy, reveal and browser fill allowed" : "Copy and reveal allowed"}</span>
              </label>
            ) : null}
          </>
        ) : null}

        {isConnectionResource ? (
          <>
            {/* Host+port and username+password are each wrapped in a full-width
                sub-grid so the pair always shares one row regardless of how the
                surrounding fields flow. */}
            <div className="wide form-grid">
              <label>
                <span>Target host</span>
                <input disabled={coreLocked} value={form.targetHost} onChange={(event) => update("targetHost", event.target.value)} />
              </label>
              <label>
                <span>Target port</span>
                <input
                  type="number"
                  disabled={coreLocked}
                  value={form.targetPort ?? ""}
                  onChange={(event) => update("targetPort", event.target.value ? Number(event.target.value) : undefined)}
                />
              </label>
            </div>
            <div className="wide form-grid">
              <label>
                <span>Username</span>
                <input disabled={coreLocked} value={form.username} onChange={(event) => update("username", event.target.value)} />
              </label>
              <SecretField
                label="Password / passphrase"
                value={form.secretValue}
                placeholder={storedSecretPlaceholder}
                disabled={isManagedExternalSource || form.secretMode === "prompt_on_launch" || coreLocked}
                visible={passwordVisible}
                revealAvailable={Boolean(resource && onRevealStoredPassword)}
                revealLoading={loading || passwordRevealLoading}
                onChange={(next) => update("secretValue", next)}
                onToggleVisibility={() => void handlePasswordVisibilityToggle()}
              />
            </div>
            <label>
              <span>Launch mode</span>
              <input disabled={coreLocked} value={form.launchMode} onChange={(event) => update("launchMode", event.target.value)} />
            </label>
            <label>
              <span>Domain</span>
              <input disabled={coreLocked} value={form.connectionDomain} onChange={(event) => update("connectionDomain", event.target.value)} />
            </label>
          </>
        ) : null}

        {form.type === "rdp" ? (
          <>
            <label>
              <span>MAC address</span>
              <input disabled={coreLocked} value={form.connectionMacAddress} onChange={(event) => update("connectionMacAddress", event.target.value)} />
            </label>
            <label>
              <span>RD Gateway host</span>
              <input
                disabled={coreLocked}
                value={form.connectionGatewayHost}
                onChange={(event) => update("connectionGatewayHost", event.target.value)}
                placeholder="gateway.example.com (blank = direct connect)"
              />
            </label>
            <label className="checkbox">
              <input
                type="checkbox"
                disabled={coreLocked}
                checked={form.connectionAdminSession}
                onChange={(event) => update("connectionAdminSession", event.target.checked)}
              />
              <span>RDP admin session mode</span>
            </label>
          </>
        ) : null}

        {isPasswordResource ? (
          <>
            {isWebPortalPassword ? (
              <>
                <label className="wide">
                  <span>Portal URL</span>
                  <input disabled={coreLocked} value={form.targetUrl} onChange={(event) => update("targetUrl", event.target.value)} placeholder="https://portal.example.com" />
                </label>
                <label className="checkbox wide">
                  <input
                    type="checkbox"
                    disabled={coreLocked}
                    checked={isPasswordlessPortal}
                    onChange={(event) =>
                      setForm((current) => ({
                        ...current,
                        secretMode: event.target.checked ? "none" : "inline",
                        secretValue: event.target.checked ? "" : current.secretValue,
                        revealAllowed: event.target.checked ? false : current.revealAllowed
                      }))
                    }
                  />
                  <span>Passwordless sign-in (SSO / emailed code — no password is stored)</span>
                </label>
              </>
            ) : null}
            {isSharedPassword && form.targetSystem ? (
              <label className="wide">
                <span>Stored system</span>
                <input disabled={coreLocked} value={form.targetSystem} onChange={(event) => update("targetSystem", event.target.value)} />
              </label>
            ) : null}
            {/* Keep this label directly before the password field below so the two share one row. */}
            <label>
              <span>Username</span>
              <input disabled={coreLocked} value={form.username} onChange={(event) => update("username", event.target.value)} />
            </label>
          </>
        ) : null}

        {form.type === "key_vault_secret" ? (
          <>
            <label>
              <span>Vault name</span>
              <input disabled={isImportedKeyVault || coreLocked} value={form.vaultName} onChange={(event) => update("vaultName", event.target.value)} />
            </label>
            <label>
              <span>Object name</span>
              <input disabled={isImportedKeyVault || coreLocked} value={form.objectName} onChange={(event) => update("objectName", event.target.value)} />
            </label>
            <label>
              <span>Object type</span>
              <input disabled={isImportedKeyVault || coreLocked} value={form.objectType} onChange={(event) => update("objectType", event.target.value)} />
            </label>
            <label>
              <span>Version</span>
              <input disabled={isImportedKeyVault || coreLocked} value={form.objectVersion} onChange={(event) => update("objectVersion", event.target.value)} />
            </label>
            <label>
              <span>Content type</span>
              <input disabled={isImportedKeyVault || coreLocked} value={form.contentType} onChange={(event) => update("contentType", event.target.value)} />
            </label>
            <label>
              <span>Expires at</span>
              <input
                type="datetime-local"
                disabled={isImportedKeyVault || coreLocked}
                value={toDateTimeInputValue(form.expiresAt)}
                onChange={(event) => update("expiresAt", fromDateTimeInputValue(event.target.value))}
              />
            </label>
          </>
        ) : null}

        {form.type === "app_registration" ? (
          <>
            <label>
              <span>Provider</span>
              <input disabled={isImportedAppRegistration || coreLocked} value={form.provider} onChange={(event) => update("provider", event.target.value)} />
            </label>
            <label>
              <span>Application ID</span>
              <input disabled={isImportedAppRegistration || coreLocked} value={form.applicationId} onChange={(event) => update("applicationId", event.target.value)} />
            </label>
            <label>
              <span>Tenant ID</span>
              <input disabled={isImportedAppRegistration || coreLocked} value={form.tenantId} onChange={(event) => update("tenantId", event.target.value)} />
            </label>
            <label>
              <span>Client ID</span>
              <input disabled={isImportedAppRegistration || coreLocked} value={form.clientId} onChange={(event) => update("clientId", event.target.value)} />
            </label>
            <label>
              <span>Credential type</span>
              <input disabled={isImportedAppRegistration || coreLocked} value={form.credentialType} onChange={(event) => update("credentialType", event.target.value)} />
            </label>
            <label>
              <span>Credential expires at</span>
              <input
                type="datetime-local"
                disabled={isImportedAppRegistration || coreLocked}
                value={toDateTimeInputValue(form.credentialExpiresAt)}
                onChange={(event) => update("credentialExpiresAt", fromDateTimeInputValue(event.target.value))}
              />
            </label>
            <label>
              <span>External display name</span>
              <input disabled={isImportedAppRegistration || coreLocked} value={form.displayNameExternal} onChange={(event) => update("displayNameExternal", event.target.value)} />
            </label>
            <label className="wide">
              <span>Linked secret ref</span>
              <input disabled={coreLocked} value={form.linkedSecretRef} onChange={(event) => update("linkedSecretRef", event.target.value)} />
            </label>
          </>
        ) : null}

        {!isConnectionResource && !isPasswordlessPortal ? (
          <SecretField
            label={isPasswordResource ? "Password" : "Secret value"}
            value={form.secretValue}
            placeholder={storedSecretPlaceholder}
            disabled={isManagedExternalSource || form.secretMode === "prompt_on_launch" || coreLocked}
            visible={passwordVisible}
            revealAvailable={Boolean(resource && onRevealStoredPassword)}
            revealLoading={loading || passwordRevealLoading}
            onChange={(next) => update("secretValue", next)}
            onToggleVisibility={() => void handlePasswordVisibilityToggle()}
          />
        ) : null}
        {!isPasswordResource ? (
          <>
            <label>
              <span>Secret mode</span>
              {renderPicker({
                open: secretModePickerOpen,
                value: form.secretMode,
                placeholder: "Select secret mode",
                options: secretModeOptions,
                disabled: isManagedExternalSource || coreLocked,
                onToggle: () => {
                  closeOtherPickers("secretMode");
                  setSecretModePickerOpen((open) => !open);
                },
                onSelect: (value) => update("secretMode", value)
              })}
            </label>
            <label className="wide">
              <span>Secret reference</span>
              <input
                disabled={isManagedExternalSource || form.secretMode === "prompt_on_launch" || coreLocked}
                value={form.secretReference}
                onChange={(event) => update("secretReference", event.target.value)}
              />
            </label>
          </>
        ) : null}
      </div>

      {coreLocked ? (
        <div className="banner compact">
          You are not the owner of this shared object, so only description, notes, folder path and environment can be changed.
        </div>
      ) : null}

      {isImportedKeyVault ? (
        <div className="banner compact">
          Azure-owned Key Vault fields are read-only here. Only workspace metadata like owner, team, notes, and allowed groups can be changed in the app.
        </div>
      ) : null}

      {isImportedAppRegistration ? (
        <div className="banner compact">
          Entra-owned app registration fields are read-only here. Workspace metadata and the optional linked secret reference can be changed in the app.
        </div>
      ) : null}

      <button
        className="button primary"
        disabled={loading}
        onClick={() => {
          // Creating a shared password is easy to do by accident (an empty
          // group list means everyone with password access can see it), so ask
          // once on first save instead of forcing personal-then-edit.
          if (!resource && isPasswordResource && !form.personal) {
            const audienceParts = [
              ...(form.allowedGroups.length > 0 ? [`members of: ${form.allowedGroups.join(", ")}`] : []),
              ...(form.allowedUsers.length > 0 ? [`users: ${form.allowedUsers.map(allowedUserLabel).join(", ")}`] : [])
            ];
            const audience =
              audienceParts.length > 0
                ? audienceParts.join(" and ")
                : "everyone with access to the Passwords module";
            const confirmedShared = window.confirm(
              `This password will be created as SHARED and visible to ${audience}.\n\n` +
                "If it is your own login, cancel and keep \"Personal saved password\" checked instead."
            );
            if (!confirmedShared) {
              return;
            }
          }
          const prepared = isPasswordResource
            ? {
                ...form,
                ownerTeam: "",
                sourceKind: "manual" as const,
                sourceObjectId: "",
                launchAllowed: isWebPortalPassword ? form.launchAllowed : false,
                // One usage permission, written to both fields. The backend
                // normalizes the same way, so neither side can drift.
                revealAllowed: isPasswordlessPortal ? false : secretUsageAllowed,
                copyAllowed: isPasswordlessPortal ? false : secretUsageAllowed,
                secretMode: isPasswordlessPortal ? ("none" as const) : ("inline" as const),
                secretValue: isPasswordlessPortal ? "" : form.secretValue,
                secretReference: ""
              }
            : form;
          void onSubmit(prepared);
        }}
      >
        {loading ? "Saving..." : resource ? "Update object" : "Create object"}
      </button>
    </section>
  );
}
