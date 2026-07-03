import { useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import type { AppRegistrationDiscoverResult, AppRegistrationImportForm, LocalGroup, UserSummary } from "../types";
import { getSelectedAppRegistrationItems, nextDiscoveredCredential, discoveredCredentialSummary } from "../appRegistration";
import { formatShortDate } from "../format";

type Props = {
  discoveries: AppRegistrationDiscoverResult;
  form: AppRegistrationImportForm;
  setForm: Dispatch<SetStateAction<AppRegistrationImportForm>>;
  knownUsers: UserSummary[];
  localGroups: LocalGroup[];
  importedAppIds: Set<string>;
  tenantLabel: string;
  authorityLabel: string;
  busy: boolean;
  onRefresh: () => void;
  onImport: () => void;
  onClose: () => void;
};

export function AppRegistrationImportModal({
  discoveries,
  form,
  setForm,
  knownUsers,
  localGroups,
  importedAppIds,
  tenantLabel,
  authorityLabel,
  busy,
  onRefresh,
  onImport,
  onClose
}: Props) {
  const [search, setSearch] = useState("");
  const [ownerOpen, setOwnerOpen] = useState(false);
  const [ownerTeamOpen, setOwnerTeamOpen] = useState(false);
  const [groupOpen, setGroupOpen] = useState(false);
  const [ownerSearch, setOwnerSearch] = useState("");
  const [ownerTeamSearch, setOwnerTeamSearch] = useState("");
  const [groupSearch, setGroupSearch] = useState("");

  const normalizedSearch = search.trim().toLowerCase();
  const filteredDiscoveries = discoveries.items.filter((item) => {
    if (normalizedSearch === "") {
      return true;
    }
    const ownerText = item.owners.map((owner) => `${owner.displayName} ${owner.email}`).join(" ");
    return (
      item.displayName.toLowerCase().includes(normalizedSearch) ||
      item.appId.toLowerCase().includes(normalizedSearch) ||
      item.id.toLowerCase().includes(normalizedSearch) ||
      item.publisherDomain.toLowerCase().includes(normalizedSearch) ||
      ownerText.toLowerCase().includes(normalizedSearch)
    );
  });
  const selectedItems = getSelectedAppRegistrationItems(form.selectedApplicationIds, discoveries.items);
  const selectedImportableAppCount = selectedItems.filter((item) => !importedAppIds.has(item.appId)).length;
  const selectedAppCredentialCount = selectedItems.reduce((sum, item) => sum + item.credentials.length, 0);
  const selectedAppOwnerCount = new Set(selectedItems.flatMap((item) => item.owners.map((owner) => owner.id))).size;

  const filteredOwners = knownUsers.filter((owner) => {
    const query = ownerSearch.trim().toLowerCase();
    if (query === "") {
      return true;
    }
    return owner.name.toLowerCase().includes(query) || owner.email.toLowerCase().includes(query);
  });
  const filteredOwnerTeams = localGroups.filter((group) =>
    group.name.toLowerCase().includes(ownerTeamSearch.trim().toLowerCase())
  );
  const filteredGroups = localGroups.filter((group) =>
    group.name.toLowerCase().includes(groupSearch.trim().toLowerCase())
  );

  function closePickers(...keepOpen: Array<"owner" | "ownerTeam" | "group">) {
    if (!keepOpen.includes("owner")) {
      setOwnerOpen(false);
    }
    if (!keepOpen.includes("ownerTeam")) {
      setOwnerTeamOpen(false);
    }
    if (!keepOpen.includes("group")) {
      setGroupOpen(false);
    }
  }

  function toggleApp(applicationId: string) {
    setForm((current) => ({
      ...current,
      selectedApplicationIds: current.selectedApplicationIds.includes(applicationId)
        ? current.selectedApplicationIds.filter((item) => item !== applicationId)
        : [...current.selectedApplicationIds, applicationId]
    }));
  }

  function setSelected(applicationIds: string[], selected: boolean) {
    setForm((current) => {
      const currentSet = new Set(current.selectedApplicationIds);
      for (const applicationId of applicationIds) {
        if (selected) {
          currentSet.add(applicationId);
        } else {
          currentSet.delete(applicationId);
        }
      }
      return { ...current, selectedApplicationIds: Array.from(currentSet) };
    });
  }

  return (
    <div className="modal-scrim" onClick={onClose}>
      <div className="modal-card keyvault-import-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <p className="eyebrow">App registrations</p>
            <h2>Import from Microsoft Entra</h2>
          </div>
          <button className="button ghost" onClick={onClose}>
            Close
          </button>
        </div>
        <div className="keyvault-import-grid">
          <section className="panel embedded-panel">
            <div className="panel-header">
              <div>
                <p className="eyebrow">Discovery</p>
                <h2>Available applications</h2>
                <p className="section-copy">Credential expiry metadata is imported without storing secret values.</p>
              </div>
              <button className="button ghost" disabled={busy} onClick={onRefresh}>
                Refresh
              </button>
            </div>
            <div className="keyvault-discovery-toolbar">
              <input
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder="Filter by app name, client ID, publisher, or owner"
              />
              <div className="resource-actions-mini">
                <span>{selectedImportableAppCount} selected</span>
                <span>{filteredDiscoveries.length} visible</span>
              </div>
            </div>
            <div className="discovery-source-list">
              <article className="group-card">
                <div className="source-summary">
                  <div>
                    <strong>{tenantLabel}</strong>
                    <p>{authorityLabel}</p>
                  </div>
                  {filteredDiscoveries.length > 0 ? (
                    <div className="panel-header-actions compact-actions">
                      <button
                        className="button ghost"
                        type="button"
                        onClick={() =>
                          setSelected(
                            filteredDiscoveries.filter((item) => !importedAppIds.has(item.appId)).map((item) => item.id),
                            true
                          )
                        }
                      >
                        Select all
                      </button>
                      <button
                        className="button ghost"
                        type="button"
                        onClick={() => setSelected(filteredDiscoveries.map((item) => item.id), false)}
                      >
                        Clear
                      </button>
                    </div>
                  ) : null}
                </div>
              </article>
              <div className="discovery-item-list">
                {filteredDiscoveries.length === 0 ? (
                  <p className="section-copy">No discoverable app registrations matched the current filter.</p>
                ) : null}
                {filteredDiscoveries.map((item) => {
                  const alreadyImported = importedAppIds.has(item.appId);
                  const nextCredential = nextDiscoveredCredential(item);
                  return (
                    <label key={item.id} className="discovery-item">
                      <input
                        type="checkbox"
                        disabled={alreadyImported}
                        checked={form.selectedApplicationIds.includes(item.id)}
                        onChange={() => toggleApp(item.id)}
                      />
                      <div>
                        <strong>{item.displayName || item.appId}</strong>
                        <p>{item.appId}</p>
                        <p>{item.publisherDomain || item.signInAudience || "no publisher domain"}</p>
                        <p>{discoveredCredentialSummary(item)}</p>
                        <p>{nextCredential?.endDateTime ? `next credential expires ${formatShortDate(nextCredential.endDateTime)}` : "no credential expiry"}</p>
                        <p>
                          {item.owners.length > 0
                            ? `owners ${item.owners.map((owner) => owner.displayName || owner.email || owner.id).join(", ")}`
                            : "no owners returned"}
                        </p>
                        {item.ownerError ? <p className="error-copy">{item.ownerError}</p> : null}
                        {alreadyImported ? <p>already imported</p> : null}
                      </div>
                    </label>
                  );
                })}
              </div>
            </div>
          </section>

          <section className="panel embedded-panel">
            <div className="panel-header">
              <div>
                <p className="eyebrow">Import settings</p>
                <h2>Ownership and visibility</h2>
                <p className="section-copy">These settings identify who should react when an app credential approaches expiry.</p>
              </div>
            </div>
            <div className="keyvault-selection-summary">
              <div className="detail-section">
                <dt>Selected apps</dt>
                <dd>{selectedImportableAppCount}</dd>
              </div>
              <div className="detail-section">
                <dt>Credentials</dt>
                <dd>{selectedAppCredentialCount}</dd>
              </div>
              <div className="detail-section">
                <dt>Azure owners</dt>
                <dd>{selectedAppOwnerCount}</dd>
              </div>
              <div className="detail-section wide">
                <dt>Selection preview</dt>
                {selectedItems.length === 0 ? (
                  <dd className="section-copy">Choose at least one app registration on the left to review and import it here.</dd>
                ) : (
                  <div className="selected-secret-list scroll-panel">
                    {selectedItems.map((item) => (
                      <button
                        key={item.id}
                        type="button"
                        className="selected-secret-row"
                        onClick={() => toggleApp(item.id)}
                      >
                        <div>
                          <strong>{item.displayName || item.appId}</strong>
                          <p>{discoveredCredentialSummary(item)}</p>
                        </div>
                        <span>Remove</span>
                      </button>
                    ))}
                  </div>
                )}
              </div>
            </div>
            <div className="form-grid">
              <label>
                <span>Owner</span>
                <div className="picker-shell">
                  <button
                    type="button"
                    className="single-picker-trigger"
                    onClick={() => {
                      closePickers("owner");
                      setOwnerOpen((open) => !open);
                    }}
                  >
                    <span>{form.owner || "Select owner"}</span>
                    <span>{ownerOpen ? "Close" : "Select"}</span>
                  </button>
                  {ownerOpen ? (
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
                            className={`picker-option ${form.owner === owner.name ? "active" : ""}`}
                            onClick={() => {
                              setForm((current) => ({ ...current, owner: owner.name }));
                              setOwnerOpen(false);
                              setOwnerSearch("");
                            }}
                          >
                            <strong>{owner.name}</strong>
                            <span>{owner.email}</span>
                          </button>
                        ))}
                        {filteredOwners.length === 0 ? (
                          <span className="selection-hint">No users match the current search.</span>
                        ) : null}
                      </div>
                    </div>
                  ) : null}
                </div>
              </label>
              <label>
                <span>Owner team</span>
                <div className="picker-shell">
                  <button
                    type="button"
                    className="single-picker-trigger"
                    onClick={() => {
                      closePickers("ownerTeam");
                      setOwnerTeamOpen((open) => !open);
                    }}
                  >
                    <span>{form.ownerTeam || "Select owner team"}</span>
                    <span>{ownerTeamOpen ? "Close" : "Select"}</span>
                  </button>
                  {ownerTeamOpen ? (
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
                            key={group.name}
                            type="button"
                            className={`picker-option ${form.ownerTeam === group.name ? "active" : ""}`}
                            onClick={() => {
                              setForm((current) => ({ ...current, ownerTeam: group.name }));
                              setOwnerTeamOpen(false);
                              setOwnerTeamSearch("");
                            }}
                          >
                            <strong>{group.name}</strong>
                          </button>
                        ))}
                        {filteredOwnerTeams.length === 0 ? (
                          <span className="selection-hint">No groups match the current search.</span>
                        ) : null}
                      </div>
                    </div>
                  ) : null}
                </div>
              </label>
              <label>
                <span>Environment</span>
                <input
                  value={form.environment}
                  onChange={(event) => setForm((current) => ({ ...current, environment: event.target.value }))}
                />
              </label>
              <label className="wide">
                <span>Description</span>
                <textarea
                  rows={3}
                  value={form.description}
                  onChange={(event) => setForm((current) => ({ ...current, description: event.target.value }))}
                />
              </label>
              <label className="wide">
                <span>Notes</span>
                <textarea
                  rows={3}
                  value={form.notes}
                  onChange={(event) => setForm((current) => ({ ...current, notes: event.target.value }))}
                />
              </label>
              <label className="wide">
                <span>Allowed groups</span>
                <div className="picker-shell">
                  <button
                    type="button"
                    className="group-picker-trigger"
                    onClick={() => {
                      closePickers("group");
                      setGroupOpen((open) => !open);
                    }}
                  >
                    <div className="group-picker-value">
                      {form.allowedGroups.length > 0 ? (
                        form.allowedGroups.map((group) => (
                          <span key={group} className="tag">
                            {group}
                          </span>
                        ))
                      ) : (
                        <span className="selection-hint">Everyone</span>
                      )}
                    </div>
                    <span>{groupOpen ? "Close" : "Select"}</span>
                  </button>
                  {groupOpen ? (
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
                              checked={form.allowedGroups.includes(group.name)}
                              onChange={() =>
                                setForm((current) => ({
                                  ...current,
                                  allowedGroups: current.allowedGroups.includes(group.name)
                                    ? current.allowedGroups.filter((item) => item !== group.name)
                                    : [...current.allowedGroups, group.name]
                                }))
                              }
                            />
                            <span>{group.name}</span>
                          </label>
                        ))}
                        {filteredGroups.length === 0 ? (
                          <span className="selection-hint">
                            {localGroups.length === 0
                              ? "No local groups available. Everyone can see imported resources."
                              : "No groups match the current search."}
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
                disabled={busy || selectedImportableAppCount === 0 || !form.owner}
                onClick={onImport}
              >
                {selectedImportableAppCount <= 1 ? "Import selected app" : `Import ${selectedImportableAppCount} apps`}
              </button>
              <button className="button ghost" onClick={onClose}>
                Cancel
              </button>
            </div>
          </section>
        </div>
      </div>
    </div>
  );
}
