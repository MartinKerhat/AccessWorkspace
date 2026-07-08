import { useEffect, useMemo, useState } from "react";
import type { CreateUserInput, LocalGroup, UserAccessDetail, UserAccessUpdateInput, UserSummary, VisibleResourceSummary } from "../types";
import { categoryLabel, type WorkspaceCategory } from "../workspaceCategories";

type Props = {
  items: UserSummary[];
  selectedId?: string;
  selectedUser?: UserAccessDetail;
  visibleResources: VisibleResourceSummary[];
  availableGroups: LocalGroup[];
  availableRights: readonly string[];
  currentUserId: string;
  loading?: boolean;
  onSelect: (id: string) => void;
  onCreate: (input: CreateUserInput) => Promise<boolean>;
  onSave: (input: UserAccessUpdateInput) => void;
  onDelete: (user: UserAccessDetail) => void;
};

const emptyCreateUserDraft: CreateUserInput = {
  username: "",
  displayName: "",
  email: "",
  password: "",
  isAdmin: false,
  blocked: false,
  directLocalGroups: []
};

const assignmentSourceLabels: Record<string, string> = {
  direct_assignment: "Direct assignment",
  mapped_external_group: "Mapped external group",
  matching_external_group_name: "Matching external group name"
};

function assignmentSourceLabel(source: string) {
  return assignmentSourceLabels[source] ?? source.split("_").join(" ");
}

function capabilityLabels(capabilities: UserAccessDetail["capabilities"]["categories"][WorkspaceCategory]) {
  return [
    capabilities.view ? "view" : "",
    capabilities.create ? "create" : "",
    capabilities.import ? "import" : "",
    capabilities.edit ? "edit" : "",
    capabilities.reveal ? "reveal" : "",
    capabilities.launch ? "launch" : ""
  ].filter(Boolean);
}

function visibilityScopeLabel(resource: VisibleResourceSummary) {
  switch (resource.visibilityScope) {
    case "administrator":
      return "Administrator access bypasses group restrictions.";
    case "everyone":
      return "Visible to everyone because no allowed groups are set.";
    case "matched_groups":
      return `Matched local groups: ${resource.matchedLocalGroups.join(", ")}.`;
    default:
      return "";
  }
}

export function UserAccessAdminPage({
  items,
  selectedId,
  selectedUser,
  visibleResources,
  availableGroups,
  availableRights,
  currentUserId,
  loading,
  onSelect,
  onCreate,
  onSave,
  onDelete
}: Props) {
  const [query, setQuery] = useState("");
  const [draftBlocked, setDraftBlocked] = useState(false);
  const [draftDirectGroups, setDraftDirectGroups] = useState<string[]>([]);
  const [draftDirectRights, setDraftDirectRights] = useState<string[]>([]);
  const [groupPickerOpen, setGroupPickerOpen] = useState(false);
  const [groupSearch, setGroupSearch] = useState("");
  const [showExternalGroups, setShowExternalGroups] = useState(false);
  const [showMapping, setShowMapping] = useState(false);
  const [showCategoryAccess, setShowCategoryAccess] = useState(false);
  const [showVisibleResources, setShowVisibleResources] = useState(false);
  const [resourceQuery, setResourceQuery] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [createDraft, setCreateDraft] = useState<CreateUserInput>(emptyCreateUserDraft);
  const [createGroupPickerOpen, setCreateGroupPickerOpen] = useState(false);
  const [createGroupSearch, setCreateGroupSearch] = useState("");

  useEffect(() => {
    setDraftBlocked(Boolean(selectedUser?.blocked));
    setDraftDirectGroups(selectedUser?.directAssignedLocalGroups ?? []);
    setDraftDirectRights(selectedUser?.directRights ?? []);
    setGroupPickerOpen(false);
    setGroupSearch("");
    setShowExternalGroups(false);
    setShowMapping(false);
    setShowCategoryAccess(false);
    setShowVisibleResources(false);
    setResourceQuery("");
  }, [selectedUser?.id, selectedUser?.blocked, selectedUser?.directAssignedLocalGroups, selectedUser?.directRights]);

  const filteredItems = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    if (normalized === "") {
      return items;
    }
    return items.filter((item) =>
      item.name.toLowerCase().includes(normalized) ||
      item.email.toLowerCase().includes(normalized) ||
      item.localGroups.some((group) => group.toLowerCase().includes(normalized))
    );
  }, [items, query]);

  const directGroupsChanged = useMemo(() => {
    const current = [...(selectedUser?.directAssignedLocalGroups ?? [])].sort().join("|");
    const draft = [...draftDirectGroups].sort().join("|");
    return current !== draft;
  }, [draftDirectGroups, selectedUser?.directAssignedLocalGroups]);

  const directRightsChanged = useMemo(() => {
    const current = [...(selectedUser?.directRights ?? [])].sort().join("|");
    const draft = [...draftDirectRights].sort().join("|");
    return current !== draft;
  }, [draftDirectRights, selectedUser?.directRights]);

  const hasChanges = selectedUser ? selectedUser.blocked !== draftBlocked || directGroupsChanged || directRightsChanged : false;

  const filteredGroups = useMemo(() => {
    const normalized = groupSearch.trim().toLowerCase();
    if (normalized === "") {
      return availableGroups;
    }
    return availableGroups.filter((group) =>
      group.name.toLowerCase().includes(normalized) ||
      group.description.toLowerCase().includes(normalized)
    );
  }, [availableGroups, groupSearch]);

  const filteredVisibleResources = useMemo(() => {
    const normalized = resourceQuery.trim().toLowerCase();
    if (normalized === "") {
      return visibleResources;
    }
    return visibleResources.filter((resource) =>
      resource.name.toLowerCase().includes(normalized) ||
      resource.category.toLowerCase().includes(normalized) ||
      resource.owner.toLowerCase().includes(normalized) ||
      resource.ownerTeam.toLowerCase().includes(normalized) ||
      resource.environment.toLowerCase().includes(normalized) ||
      resource.targetHost.toLowerCase().includes(normalized) ||
      resource.vaultName.toLowerCase().includes(normalized) ||
      resource.objectName.toLowerCase().includes(normalized)
    );
  }, [resourceQuery, visibleResources]);

  const filteredCreateGroups = useMemo(() => {
    const normalized = createGroupSearch.trim().toLowerCase();
    if (normalized === "") {
      return availableGroups;
    }
    return availableGroups.filter((group) =>
      group.name.toLowerCase().includes(normalized) ||
      group.description.toLowerCase().includes(normalized)
    );
  }, [availableGroups, createGroupSearch]);

  const visibleResourceCounts = useMemo(() => {
    const counts: Record<WorkspaceCategory, number> = {
      connections: 0,
      keyvault: 0,
      appregistrations: 0,
      passwords: 0
    };
    for (const resource of visibleResources) {
      if (resource.category in counts) {
        counts[resource.category as WorkspaceCategory] += 1;
      }
    }
    return counts;
  }, [visibleResources]);

  function toggleDirectGroup(groupName: string) {
    setDraftDirectGroups((current) =>
      current.includes(groupName)
        ? current.filter((item) => item !== groupName)
        : [...current, groupName]
    );
  }

  function toggleDirectRight(right: string) {
    setDraftDirectRights((current) =>
      current.includes(right)
        ? current.filter((item) => item !== right)
        : [...current, right]
    );
  }

  function toggleCreateDirectGroup(groupName: string) {
    setCreateDraft((current) => ({
      ...current,
      directLocalGroups: current.directLocalGroups.includes(groupName)
        ? current.directLocalGroups.filter((item) => item !== groupName)
        : [...current.directLocalGroups, groupName]
    }));
  }

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <p className="eyebrow">Administration</p>
          <h2>User access</h2>
          <p className="section-copy">
            Inspect who can enter the workspace, see why they have access, adjust direct local-group assignments, grant user-specific rights, and block workspace sign-in when needed.
          </p>
        </div>
        <div className="panel-header-actions">
          <span className="muted">{items.length} users</span>
          <button
            type="button"
            className="button ghost compact-button"
            onClick={() => {
              setCreateOpen((open) => !open);
              if (createOpen) {
                setCreateDraft(emptyCreateUserDraft);
                setCreateGroupPickerOpen(false);
                setCreateGroupSearch("");
              }
            }}
          >
            {createOpen ? "Close create user" : "Create user"}
          </button>
        </div>
      </div>

      {createOpen ? (
        <div className="detail-section">
          <div className="panel-header compact-panel-header">
            <div>
              <p className="eyebrow">Local user</p>
              <h3>Create workspace user</h3>
              <p className="section-copy">Create a local workspace account for direct sign-in without waiting for first Entra login.</p>
            </div>
          </div>
          <div className="form-grid">
            <label>
              <span>Display name</span>
              <input
                value={createDraft.displayName}
                onChange={(event) => setCreateDraft((current) => ({ ...current, displayName: event.target.value }))}
              />
            </label>
            <label>
              <span>Email</span>
              <input
                value={createDraft.email}
                onChange={(event) => setCreateDraft((current) => ({ ...current, email: event.target.value }))}
              />
            </label>
            <label>
              <span>Username</span>
              <input
                value={createDraft.username}
                onChange={(event) => setCreateDraft((current) => ({ ...current, username: event.target.value }))}
              />
            </label>
            <label>
              <span>Password</span>
              <input
                type="password"
                value={createDraft.password}
                onChange={(event) => setCreateDraft((current) => ({ ...current, password: event.target.value }))}
              />
            </label>
            <label className="checkbox">
              <input
                type="checkbox"
                checked={createDraft.isAdmin}
                onChange={(event) => setCreateDraft((current) => ({ ...current, isAdmin: event.target.checked }))}
              />
              <span>Administrator access</span>
            </label>
            <label className="checkbox">
              <input
                type="checkbox"
                checked={createDraft.blocked}
                onChange={(event) => setCreateDraft((current) => ({ ...current, blocked: event.target.checked }))}
              />
              <span>Create as blocked</span>
            </label>
            <label className="wide">
              <span>Direct local groups</span>
              <div className="picker-shell">
                <button
                  type="button"
                  className="group-picker-trigger"
                  onClick={() => setCreateGroupPickerOpen((open) => !open)}
                >
                  <div className="group-picker-value">
                    {createDraft.directLocalGroups.length > 0 ? (
                      createDraft.directLocalGroups.map((group) => (
                        <span key={group} className="tag">
                          {group}
                        </span>
                      ))
                    ) : (
                      <span className="selection-hint">No direct local groups selected</span>
                    )}
                  </div>
                  <span>{createGroupPickerOpen ? "Close" : "Select"}</span>
                </button>
                {createGroupPickerOpen ? (
                  <div className="group-picker-dropdown">
                    <input
                      className="picker-search-input"
                      value={createGroupSearch}
                      onChange={(event) => setCreateGroupSearch(event.target.value)}
                      placeholder="Search local groups"
                    />
                    <div className="picker-option-list">
                      {filteredCreateGroups.map((group) => (
                        <label key={group.name} className="checkbox">
                          <input
                            type="checkbox"
                            checked={createDraft.directLocalGroups.includes(group.name)}
                            onChange={() => toggleCreateDirectGroup(group.name)}
                          />
                          <span>
                            <strong>{group.name}</strong>
                            <small>{group.description || "No description"}</small>
                          </span>
                        </label>
                      ))}
                      {filteredCreateGroups.length === 0 ? (
                        <span className="selection-hint">
                          {availableGroups.length === 0 ? "No local groups available yet." : "No groups match the current search."}
                        </span>
                      ) : null}
                    </div>
                  </div>
                ) : null}
              </div>
            </label>
          </div>
          <div className="action-row">
            <button
              className="button primary"
              disabled={loading}
              onClick={() => {
                void onCreate(createDraft).then((created) => {
                  if (!created) {
                    return;
                  }
                  setCreateDraft(emptyCreateUserDraft);
                  setCreateOpen(false);
                  setCreateGroupPickerOpen(false);
                  setCreateGroupSearch("");
                });
              }}
            >
              Create user
            </button>
          </div>
        </div>
      ) : null}

      <div className="user-admin-grid">
        <div className="user-admin-column">
          <input
            placeholder="Search users, email, local groups"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
          <div className="user-admin-list">
            {filteredItems.length === 0 ? (
              <div className="detail-section">
                <p className="eyebrow">No users</p>
                <p className="section-copy">No users match the current search.</p>
              </div>
            ) : null}
            {filteredItems.map((item) => (
              <button
                key={item.id}
                type="button"
                className={`user-access-row ${selectedId === item.id ? "active" : ""}`}
                onClick={() => onSelect(item.id)}
              >
                <div className="user-access-row-top">
                  <strong>{item.name}</strong>
                  <div className="tag-row">
                    {item.isAdmin ? <span className="tag">admin</span> : null}
                    {item.blocked ? <span className="tag blocked-tag">blocked</span> : null}
                  </div>
                </div>
                <p>{item.email}</p>
                <div className="meta-row">
                  <span>{item.localGroups.length} local groups</span>
                  <span>{item.rightsCount} rights</span>
                </div>
              </button>
            ))}
          </div>
        </div>

        <div className="user-admin-column">
          {!selectedUser ? (
            <div className="detail-section">
              <p className="eyebrow">User details</p>
              <h3>Select a user</h3>
              <p className="section-copy">Pick a user from the list to review effective access and manage direct local groups and user-specific rights.</p>
            </div>
          ) : (
            <>
              <div className="panel-header">
                <div className="detail-header-copy">
                  <p className="eyebrow">User details</p>
                  <h2>{selectedUser.name}</h2>
                  <p className="detail-description">{selectedUser.email}</p>
                </div>
                <div className="detail-header-actions user-access-header-actions">
                  {selectedUser.isAdmin ? <span className="resource-type">Administrator</span> : null}
                  <label className="toggle-chip">
                    <input
                      type="checkbox"
                      checked={draftBlocked}
                      onChange={(event) => setDraftBlocked(event.target.checked)}
                    />
                    <span>Blocked</span>
                  </label>
                  <button
                    className="button primary"
                    disabled={loading || !hasChanges}
                    onClick={() =>
                      onSave({
                        blocked: draftBlocked,
                        directLocalGroups: draftDirectGroups,
                        directRights: draftDirectRights
                      })
                    }
                  >
                    Save user access
                  </button>
                  <button
                    className="button ghost danger-button"
                    disabled={loading || selectedUser.id === currentUserId}
                    title={
                      selectedUser.id === currentUserId
                        ? "You cannot delete your own account"
                        : "Delete this user and all of their personal saved passwords"
                    }
                    onClick={() => onDelete(selectedUser)}
                  >
                    Delete user
                  </button>
                </div>
              </div>

              {selectedUser.id === currentUserId && draftBlocked ? (
                <p className="error-copy">Blocking this user will end the current admin session after save.</p>
              ) : null}

              {selectedUser.id !== currentUserId ? (
                <p className="section-copy">
                  Deleting this user permanently removes their account and cascades a cleanup of all of their personal saved
                  passwords from the database. Personal secrets are never visible to anyone but their owner, so they cannot be
                  transferred — they are deleted with the account.
                </p>
              ) : null}

              <dl className="detail-grid">
                <div>
                  <dt>External groups</dt>
                  <dd>{selectedUser.externalGroups.length}</dd>
                </div>
                <div>
                  <dt>Resolved local groups</dt>
                  <dd>{selectedUser.resolvedLocalGroups.length}</dd>
                </div>
                <div>
                  <dt>Rights</dt>
                  <dd>{selectedUser.rights.length}</dd>
                </div>
                <div>
                  <dt>Direct rights</dt>
                  <dd>{draftDirectRights.length}</dd>
                </div>
                <div>
                  <dt>Workspace access</dt>
                  <dd>{draftBlocked ? "blocked" : "allowed"}</dd>
                </div>
              </dl>

              <div className="group-card-section">
                <div className="panel-header compact-panel-header">
                  <div>
                    <p className="eyebrow">Direct local groups</p>
                    <p className="section-copy">Use direct assignment when you want a user to have workspace access without relying only on Azure group mapping.</p>
                  </div>
                </div>
                <div className="picker-shell">
                  <button
                    type="button"
                    className="group-picker-trigger"
                    onClick={() => setGroupPickerOpen((open) => !open)}
                  >
                    <div className="group-picker-value">
                      {draftDirectGroups.length > 0 ? (
                        draftDirectGroups.map((group) => (
                          <span key={group} className="tag">
                            {group}
                          </span>
                        ))
                      ) : (
                        <span className="selection-hint">No direct local groups selected</span>
                      )}
                    </div>
                    <span>{groupPickerOpen ? "Close" : "Select"}</span>
                  </button>
                  {groupPickerOpen ? (
                    <div className="group-picker-dropdown">
                      <input
                        className="picker-search-input"
                        value={groupSearch}
                        onChange={(event) => setGroupSearch(event.target.value)}
                        placeholder="Search local groups"
                      />
                      <div className="picker-option-list">
                        {filteredGroups.map((group) => (
                          <label key={group.name} className="checkbox">
                            <input
                              type="checkbox"
                              checked={draftDirectGroups.includes(group.name)}
                              onChange={() => toggleDirectGroup(group.name)}
                            />
                            <span>
                              <strong>{group.name}</strong>
                              <small>{group.description || "No description"}</small>
                            </span>
                          </label>
                        ))}
                        {filteredGroups.length === 0 ? (
                          <span className="selection-hint">
                            {availableGroups.length === 0 ? "No local groups available yet." : "No groups match the current search."}
                          </span>
                        ) : null}
                      </div>
                    </div>
                  ) : null}
                </div>
              </div>

              <div className="group-card-section">
                <div className="panel-header compact-panel-header">
                  <div>
                    <p className="eyebrow">Direct rights</p>
                    <p className="section-copy">Grant user-specific rights here when group membership alone is not the right scope.</p>
                  </div>
                </div>
                <div className="group-rights-grid">
                  {availableRights.map((right) => (
                    <label key={right} className="checkbox">
                      <input
                        type="checkbox"
                        checked={draftDirectRights.includes(right)}
                        onChange={() => toggleDirectRight(right)}
                      />
                      <span>{right}</span>
                    </label>
                  ))}
                </div>
              </div>

              <div className="group-card-section">
                <div className="panel-header compact-panel-header">
                  <div>
                    <p className="eyebrow">External groups</p>
                    <p className="section-copy">Large Azure group sets stay collapsed by default so the detail view remains readable.</p>
                  </div>
                  <button className="button ghost compact-button" onClick={() => setShowExternalGroups((open) => !open)}>
                    {showExternalGroups ? "Hide groups" : `Show groups (${selectedUser.externalGroups.length})`}
                  </button>
                </div>
                {showExternalGroups ? (
                  <div className="tag-row">
                    {selectedUser.externalGroups.map((group) => (
                      <span key={group} className="tag">
                        {group}
                      </span>
                    ))}
                  </div>
                ) : null}
              </div>

              <div className="group-card-section">
                <div className="panel-header compact-panel-header">
                  <div>
                    <p className="eyebrow">Resolved local groups</p>
                    <p className="section-copy">These are the local groups the workspace resolved for this user.</p>
                  </div>
                  <button className="button ghost compact-button" onClick={() => setShowMapping((open) => !open)}>
                    {showMapping ? "Hide mapping" : "Show mapping"}
                  </button>
                </div>
                {selectedUser.resolvedLocalGroups.length === 0 ? (
                  <p className="section-copy">No local group currently resolves for this user.</p>
                ) : (
                  <div className="user-access-group-list">
                    {selectedUser.resolvedLocalGroups.map((group) => (
                      <div key={`${selectedUser.id}-${group.name}`} className="user-access-group-item">
                        <div className="user-access-row-top">
                          <strong>{group.name}</strong>
                          {showMapping ? <span className="tag">{assignmentSourceLabel(group.assignmentSource)}</span> : null}
                        </div>
                        {showMapping && group.matchedExternalGroup ? (
                          <p className="section-copy">Matched through {group.matchedExternalGroup}</p>
                        ) : null}
                        <div className="tag-row">
                          {group.rights.map((right) => (
                            <span key={`${group.name}-${right}`} className="tag">
                              {right}
                            </span>
                          ))}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              <div className="group-card-section">
                <div className="panel-header compact-panel-header">
                  <div>
                    <p className="eyebrow">Effective category access</p>
                    <p className="section-copy">These are the workspace category capabilities currently resolved for this user.</p>
                  </div>
                  <button className="button ghost compact-button" onClick={() => setShowCategoryAccess((open) => !open)}>
                    {showCategoryAccess ? "Hide access" : "Show access"}
                  </button>
                </div>
                {showCategoryAccess ? (
                  <div className="user-access-capabilities">
                    {(["connections", "keyvault", "appregistrations", "passwords"] as WorkspaceCategory[]).map((category) => {
                      const capabilities = selectedUser.capabilities.categories[category];
                      const labels = capabilities ? capabilityLabels(capabilities) : [];
                      return (
                        <div key={category} className="user-access-capability">
                          <strong>{categoryLabel(category)}</strong>
                          {labels.length > 0 ? (
                            <div className="tag-row">
                              {labels.map((label) => (
                                <span key={`${category}-${label}`} className="tag">
                                  {label}
                                </span>
                              ))}
                            </div>
                          ) : (
                            <p className="section-copy">No access</p>
                          )}
                        </div>
                      );
                    })}
                  </div>
                ) : null}
              </div>

              <div className="group-card-section">
                <div className="panel-header compact-panel-header">
                  <div>
                    <p className="eyebrow">Visible resources</p>
                    <p className="section-copy">This is the current catalog view the selected user can access after category rights and allowed-group filtering are both applied.</p>
                  </div>
                  <button className="button ghost compact-button" onClick={() => setShowVisibleResources((open) => !open)}>
                    {showVisibleResources ? "Hide resources" : `Show resources (${visibleResources.length})`}
                  </button>
                </div>
                <dl className="detail-grid">
                  <div>
                    <dt>Total visible</dt>
                    <dd>{visibleResources.length}</dd>
                  </div>
                  <div>
                    <dt>Connections</dt>
                    <dd>{visibleResourceCounts.connections}</dd>
                  </div>
                  <div>
                    <dt>Key Vault</dt>
                    <dd>{visibleResourceCounts.keyvault}</dd>
                  </div>
                  <div>
                    <dt>App registrations</dt>
                    <dd>{visibleResourceCounts.appregistrations}</dd>
                  </div>
                  <div>
                    <dt>Passwords</dt>
                    <dd>{visibleResourceCounts.passwords}</dd>
                  </div>
                </dl>
                {showVisibleResources ? (
                  <>
                    <input
                      placeholder="Search visible resources, owner, team, environment"
                      value={resourceQuery}
                      onChange={(event) => setResourceQuery(event.target.value)}
                    />
                    {filteredVisibleResources.length === 0 ? (
                      <p className="section-copy">
                        {visibleResources.length === 0
                          ? "This user cannot currently see any active resources."
                          : "No visible resources match the current search."}
                      </p>
                    ) : (
                      <div className="user-access-group-list scroll-panel">
                        {filteredVisibleResources.map((resource) => (
                          <div key={resource.id} className="user-access-group-item">
                            <div className="user-access-row-top">
                              <strong>{resource.name}</strong>
                              <div className="tag-row">
                                <span className="tag">{categoryLabel(resource.category as WorkspaceCategory)}</span>
                                {resource.status ? <span className="tag">{resource.status}</span> : null}
                              </div>
                            </div>
                            <p className="section-copy">
                              {resource.owner}
                              {resource.ownerTeam ? ` · ${resource.ownerTeam}` : ""}
                              {resource.environment ? ` · ${resource.environment}` : ""}
                            </p>
                            <p className="section-copy">
                              Category access via {resource.categoryAccessRight || "workspace policy"} · {visibilityScopeLabel(resource)}
                            </p>
                            <div className="tag-row">
                              {resource.allowedGroups.length > 0 ? (
                                resource.allowedGroups.map((group) => (
                                  <span key={`${resource.id}-${group}`} className="tag">
                                    {group}
                                  </span>
                                ))
                              ) : (
                                <span className="tag">Everyone</span>
                              )}
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </>
                ) : null}
              </div>
            </>
          )}
        </div>
      </div>
    </section>
  );
}
