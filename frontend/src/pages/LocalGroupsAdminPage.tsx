import { useEffect, useMemo, useState } from "react";
import type { LocalGroup, LocalGroupForm, UserSummary } from "../types";

type Props = {
  items: LocalGroup[];
  availableRights: readonly string[];
  availableUsers: UserSummary[];
  loading?: boolean;
  onSave: (mode: "create" | "edit", originalName: string | undefined, input: LocalGroupForm) => Promise<void>;
};

function emptyLocalGroupForm(): LocalGroupForm {
  return {
    name: "",
    description: "",
    rights: [],
    mappedExternalGroups: [],
    assignedUserIds: []
  };
}

function toLocalGroupForm(group?: LocalGroup): LocalGroupForm {
  if (!group) {
    return emptyLocalGroupForm();
  }
  return {
    name: group.name,
    description: group.description,
    rights: [...group.rights],
    mappedExternalGroups: [...group.mappedExternalGroups],
    assignedUserIds: [...group.assignedUserIds]
  };
}

function normalizeList(values: string[]): string[] {
  return Array.from(
    new Set(
      values
        .map((value) => value.trim())
        .filter(Boolean)
    )
  );
}

function formsEqual(a: LocalGroupForm, b: LocalGroupForm): boolean {
  return (
    a.name.trim() === b.name.trim() &&
    a.description.trim() === b.description.trim() &&
    normalizeList(a.rights).sort().join("|") === normalizeList(b.rights).sort().join("|") &&
    normalizeList(a.mappedExternalGroups).sort().join("|") === normalizeList(b.mappedExternalGroups).sort().join("|") &&
    normalizeList(a.assignedUserIds).sort().join("|") === normalizeList(b.assignedUserIds).sort().join("|")
  );
}

export function LocalGroupsAdminPage({ items, availableRights, availableUsers, loading, onSave }: Props) {
  const [query, setQuery] = useState("");
  const [mode, setMode] = useState<"create" | "edit">("edit");
  const [selectedName, setSelectedName] = useState<string>();
  const [draft, setDraft] = useState<LocalGroupForm>(emptyLocalGroupForm());
  const [userPickerOpen, setUserPickerOpen] = useState(false);
  const [userSearch, setUserSearch] = useState("");

  useEffect(() => {
    if (mode === "create") {
      return;
    }
    if (selectedName) {
      const selectedGroup = items.find((item) => item.name === selectedName);
      if (selectedGroup) {
        setDraft(toLocalGroupForm(selectedGroup));
        return;
      }
    }
    if (items[0]) {
      setSelectedName(items[0].name);
      setDraft(toLocalGroupForm(items[0]));
      setMode("edit");
      return;
    }
    setSelectedName(undefined);
    setDraft(emptyLocalGroupForm());
    setMode("create");
  }, [items, mode, selectedName]);

  useEffect(() => {
    setUserPickerOpen(false);
    setUserSearch("");
  }, [mode, selectedName]);

  const filteredItems = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    if (normalized === "") {
      return items;
    }
    return items.filter((item) =>
      item.name.toLowerCase().includes(normalized) ||
      item.description.toLowerCase().includes(normalized) ||
      item.rights.some((right) => right.toLowerCase().includes(normalized)) ||
      item.mappedExternalGroups.some((group) => group.toLowerCase().includes(normalized))
    );
  }, [items, query]);

  const baseline = useMemo(() => {
    if (mode !== "edit" || !selectedName) {
      return undefined;
    }
    return items.find((item) => item.name === selectedName);
  }, [items, mode, selectedName]);

  const hasChanges = useMemo(() => {
    if (mode === "create") {
      return (
        draft.name.trim() !== "" ||
        draft.description.trim() !== "" ||
        draft.rights.length > 0 ||
        draft.mappedExternalGroups.length > 0 ||
        draft.assignedUserIds.length > 0
      );
    }
    return baseline ? !formsEqual(draft, toLocalGroupForm(baseline)) : false;
  }, [baseline, draft, mode]);

  const userByAssignment = useMemo(() => {
    const map = new Map<string, UserSummary>();
    for (const user of availableUsers) {
      map.set(user.id, user);
      map.set(user.email, user);
    }
    return map;
  }, [availableUsers]);

  const selectedKnownEntries = useMemo(
    () => draft.assignedUserIds.filter((entry) => userByAssignment.has(entry)),
    [draft.assignedUserIds, userByAssignment]
  );

  const selectedUsers = useMemo(() => {
    const seen = new Set<string>();
    const items = [] as UserSummary[];
    for (const entry of draft.assignedUserIds) {
      const user = userByAssignment.get(entry);
      if (!user || seen.has(user.id)) {
        continue;
      }
      seen.add(user.id);
      items.push(user);
    }
    return items;
  }, [draft.assignedUserIds, userByAssignment]);

  const manualAssignedEntries = useMemo(
    () => draft.assignedUserIds.filter((entry) => !userByAssignment.has(entry)),
    [draft.assignedUserIds, userByAssignment]
  );

  const filteredUsers = useMemo(() => {
    const normalized = userSearch.trim().toLowerCase();
    if (normalized === "") {
      return availableUsers;
    }
    return availableUsers.filter((user) =>
      user.name.toLowerCase().includes(normalized) ||
      user.email.toLowerCase().includes(normalized) ||
      user.localGroups.some((group) => group.toLowerCase().includes(normalized))
    );
  }, [availableUsers, userSearch]);

  function openCreate() {
    setMode("create");
    setSelectedName(undefined);
    setDraft(emptyLocalGroupForm());
  }

  function openEdit(group: LocalGroup) {
    setMode("edit");
    setSelectedName(group.name);
    setDraft(toLocalGroupForm(group));
  }

  function toggleRight(right: string) {
    setDraft((current) => ({
      ...current,
      rights: current.rights.includes(right)
        ? current.rights.filter((item) => item !== right)
        : [...current.rights, right]
    }));
  }

  function isUserAssigned(user: UserSummary) {
    return draft.assignedUserIds.includes(user.id) || draft.assignedUserIds.includes(user.email);
  }

  function toggleAssignedUser(user: UserSummary) {
    setDraft((current) => {
      const remaining = current.assignedUserIds.filter((item) => item !== user.id && item !== user.email);
      if (current.assignedUserIds.includes(user.id) || current.assignedUserIds.includes(user.email)) {
        return {
          ...current,
          assignedUserIds: remaining
        };
      }
      return {
        ...current,
        assignedUserIds: [...remaining, user.id]
      };
    });
  }

  function updateManualAssignedUsers(value: string) {
    const manualEntries = value
      .split(/\r?\n|,/)
      .map((item) => item.trim())
      .filter(Boolean);
    setDraft((current) => ({
      ...current,
      assignedUserIds: [...selectedKnownEntries, ...manualEntries]
    }));
  }

  async function save() {
    const normalizedDraft: LocalGroupForm = {
      name: draft.name.trim(),
      description: draft.description.trim(),
      rights: normalizeList(draft.rights),
      mappedExternalGroups: normalizeList(draft.mappedExternalGroups),
      assignedUserIds: normalizeList(draft.assignedUserIds)
    };
    try {
      await onSave(mode, mode === "edit" ? selectedName : undefined, normalizedDraft);
      if (mode === "create") {
        setMode("edit");
        setSelectedName(normalizedDraft.name);
      }
    } catch {
      // Parent already reports the save failure in the shared banner.
    }
  }

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <p className="eyebrow">Authorization</p>
          <h2>Local groups</h2>
          <p className="section-copy">
            Local groups are the app authorization unit. They carry rights, can be mapped to Azure group IDs, and are also used for resource visibility inside each category.
          </p>
        </div>
        <div className="panel-header-actions">
          <span className="muted">{items.length} groups</span>
          <button className="button primary" onClick={openCreate}>
            Create group
          </button>
        </div>
      </div>

      <div className="user-admin-grid">
        <div className="user-admin-column">
          <input
            placeholder="Search groups, rights, mapped IDs"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
          <div className="user-admin-list">
            {filteredItems.length === 0 ? (
              <div className="detail-section">
                <p className="eyebrow">No groups</p>
                <p className="section-copy">No local groups match the current search.</p>
              </div>
            ) : null}
            {filteredItems.map((group) => {
              const active = mode === "edit" && selectedName === group.name;
              return (
                <button
                  key={group.name}
                  type="button"
                  className={`user-access-row ${active ? "active" : ""}`}
                  onClick={() => openEdit(group)}
                >
                  <div className="user-access-row-top">
                    <strong>{group.name}</strong>
                    {group.rights.includes("admin.access") ? <span className="tag">admin access</span> : null}
                  </div>
                  <p>{group.description || "No description"}</p>
                  <div className="meta-row">
                    <span>{group.rights.length} rights</span>
                    <span>{group.mappedExternalGroups.length} mapped IDs</span>
                    <span>{group.assignedUserIds.length} direct users</span>
                  </div>
                </button>
              );
            })}
          </div>
        </div>

        <div className="user-admin-column">
          <div className="panel-header">
            <div className="detail-header-copy">
              <p className="eyebrow">Group details</p>
              <h2>{mode === "create" ? "Create local group" : draft.name || "Select a group"}</h2>
              <p className="detail-description">
                {mode === "create"
                  ? "Define the rights and assignment rules for a new local group."
                  : draft.description || "Describe what this group should be used for."}
              </p>
            </div>
            <div className="detail-header-actions">
              {mode === "create" ? <span className="resource-type">New group</span> : null}
              <button className="button primary" disabled={loading || draft.name.trim() === "" || !hasChanges} onClick={() => void save()}>
                {mode === "create" ? "Create local group" : "Save local group"}
              </button>
            </div>
          </div>

          <dl className="detail-grid">
            <div>
              <dt>Rights</dt>
              <dd>{draft.rights.length}</dd>
            </div>
            <div>
              <dt>Mapped Azure group IDs</dt>
              <dd>{draft.mappedExternalGroups.length}</dd>
            </div>
            <div>
              <dt>Direct assigned users</dt>
              <dd>{draft.assignedUserIds.length}</dd>
            </div>
            <div>
              <dt>Admin access</dt>
              <dd>{draft.rights.includes("admin.access") ? "enabled" : "not granted"}</dd>
            </div>
          </dl>

          <div className="form-grid">
            <label className="wide">
              <span>Name</span>
              <input
                value={draft.name}
                disabled={mode === "edit"}
                onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))}
                placeholder="engineering"
              />
            </label>

            <label className="wide">
              <span>Description</span>
              <textarea
                rows={3}
                value={draft.description}
                onChange={(event) => setDraft((current) => ({ ...current, description: event.target.value }))}
              />
            </label>

            <label className="wide">
              <span>Mapped Azure group IDs</span>
              <textarea
                rows={4}
                value={draft.mappedExternalGroups.join("\n")}
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    mappedExternalGroups: event.target.value.split(/\r?\n|,/).map((item) => item.trim()).filter(Boolean)
                  }))
                }
                placeholder="One Azure group ID per line"
              />
            </label>

            <label className="wide">
              <span>Direct assigned users</span>
              <div className="picker-shell">
                <button
                  type="button"
                  className="group-picker-trigger"
                  onClick={() => setUserPickerOpen((open) => !open)}
                >
                  <div className="group-picker-value">
                    {selectedUsers.length > 0 || manualAssignedEntries.length > 0 ? (
                      <>
                        {selectedUsers.map((user) => (
                          <span key={user.id} className="tag">
                            {user.name}
                          </span>
                        ))}
                        {manualAssignedEntries.map((entry) => (
                          <span key={entry} className="tag">
                            {entry}
                          </span>
                        ))}
                      </>
                    ) : (
                      <span className="selection-hint">No direct users selected</span>
                    )}
                  </div>
                  <span>{userPickerOpen ? "Close" : "Select"}</span>
                </button>
                {userPickerOpen ? (
                  <div className="group-picker-dropdown">
                    <input
                      className="picker-search-input"
                      value={userSearch}
                      onChange={(event) => setUserSearch(event.target.value)}
                      placeholder="Search users"
                    />
                    <div className="picker-option-list">
                      {filteredUsers.map((user) => (
                        <label key={user.id} className="checkbox">
                          <input
                            type="checkbox"
                            checked={isUserAssigned(user)}
                            onChange={() => toggleAssignedUser(user)}
                          />
                          <span>
                            <strong>{user.name}</strong>
                            <small>{user.email}</small>
                          </span>
                        </label>
                      ))}
                      {filteredUsers.length === 0 ? (
                        <span className="selection-hint">
                          {availableUsers.length === 0 ? "No workspace users are available yet." : "No users match the current search."}
                        </span>
                      ) : null}
                    </div>
                  </div>
                ) : null}
              </div>
              <textarea
                rows={manualAssignedEntries.length > 0 ? 3 : 2}
                value={manualAssignedEntries.join("\n")}
                onChange={(event) => updateManualAssignedUsers(event.target.value)}
                placeholder="Optional raw user IDs or emails, one per line"
              />
            </label>

            <div className="wide group-rights-grid">
              {availableRights.map((right) => (
                <label key={right} className="checkbox">
                  <input
                    type="checkbox"
                    checked={draft.rights.includes(right)}
                    onChange={() => toggleRight(right)}
                  />
                  <span>{right}</span>
                </label>
              ))}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
