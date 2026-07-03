import { useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import type { KeyVaultDiscoverResult, KeyVaultImportForm, LocalGroup, UserSummary } from "../types";
import { getSelectedKeyVaultItems } from "../keyVault";

type Props = {
  discoveries: KeyVaultDiscoverResult;
  form: KeyVaultImportForm;
  setForm: Dispatch<SetStateAction<KeyVaultImportForm>>;
  knownUsers: UserSummary[];
  localGroups: LocalGroup[];
  busy: boolean;
  onRefresh: () => void;
  onImport: () => void;
  onClose: () => void;
};

export function KeyVaultImportModal({
  discoveries,
  form,
  setForm,
  knownUsers,
  localGroups,
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
  const filteredDiscoveries = discoveries.sources.map((source) => ({
    ...source,
    items: source.items.filter((item) => {
      if (normalizedSearch === "") {
        return true;
      }
      return (
        item.name.toLowerCase().includes(normalizedSearch) ||
        item.id.toLowerCase().includes(normalizedSearch) ||
        item.vaultName.toLowerCase().includes(normalizedSearch) ||
        item.contentType.toLowerCase().includes(normalizedSearch)
      );
    })
  }));
  const selectedItems = getSelectedKeyVaultItems(form.selectedSecretIds, discoveries.sources);
  const selectedVaultCount = new Set(selectedItems.map((item) => item.vaultUrl)).size;

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

  function toggleSecret(secretId: string) {
    setForm((current) => ({
      ...current,
      selectedSecretIds: current.selectedSecretIds.includes(secretId)
        ? current.selectedSecretIds.filter((item) => item !== secretId)
        : [...current.selectedSecretIds, secretId]
    }));
  }

  function setSourceSecrets(secretIds: string[], selected: boolean) {
    setForm((current) => {
      const currentSet = new Set(current.selectedSecretIds);
      for (const secretId of secretIds) {
        if (selected) {
          currentSet.add(secretId);
        } else {
          currentSet.delete(secretId);
        }
      }
      return { ...current, selectedSecretIds: Array.from(currentSet) };
    });
  }

  function toggleAllowedGroup(group: string) {
    setForm((current) => ({
      ...current,
      allowedGroups: current.allowedGroups.includes(group)
        ? current.allowedGroups.filter((item) => item !== group)
        : [...current.allowedGroups, group]
    }));
  }

  return (
    <div className="modal-scrim" onClick={onClose}>
      <div className="modal-card keyvault-import-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <p className="eyebrow">Key Vault</p>
            <h2>Import secrets from Azure</h2>
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
                <h2>Available secrets</h2>
                <p className="section-copy">Select one or many secrets, then apply shared workspace metadata once.</p>
              </div>
              <button className="button ghost" disabled={busy} onClick={onRefresh}>
                Refresh
              </button>
            </div>
            <div className="keyvault-discovery-toolbar">
              <input
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder="Filter by secret name, vault, content type, or reference"
              />
              <div className="resource-actions-mini">
                <span>{selectedItems.length} selected</span>
                <span>{filteredDiscoveries.reduce((sum, source) => sum + source.items.length, 0)} visible</span>
              </div>
            </div>
            <div className="discovery-source-list">
              {filteredDiscoveries.map((sourceResult) => (
                <article key={sourceResult.source.vaultUrl} className="group-card">
                  <div className="group-card-section">
                    <div className="source-summary">
                      <div>
                        <strong>{sourceResult.source.name || sourceResult.source.vaultUrl}</strong>
                        <p>{sourceResult.source.vaultUrl}</p>
                      </div>
                      {sourceResult.items.length > 0 ? (
                        <div className="panel-header-actions compact-actions">
                          <button
                            className="button ghost"
                            type="button"
                            onClick={() => setSourceSecrets(sourceResult.items.map((item) => item.id), true)}
                          >
                            Select all
                          </button>
                          <button
                            className="button ghost"
                            type="button"
                            onClick={() => setSourceSecrets(sourceResult.items.map((item) => item.id), false)}
                          >
                            Clear
                          </button>
                        </div>
                      ) : null}
                    </div>
                    {sourceResult.error ? <p className="error-copy">{sourceResult.error}</p> : null}
                  </div>
                  <div className="discovery-item-list">
                    {sourceResult.items.length === 0 && !sourceResult.error ? (
                      <p className="section-copy">No discoverable secrets matched the current filter.</p>
                    ) : null}
                    {sourceResult.items.map((item) => (
                      <label key={item.id} className="discovery-item">
                        <input
                          type="checkbox"
                          checked={form.selectedSecretIds.includes(item.id)}
                          onChange={() => toggleSecret(item.id)}
                        />
                        <div>
                          <strong>{item.name}</strong>
                          <p>{item.contentType || "no content type"}</p>
                          <p>{item.enabled ? "enabled" : "disabled in Azure"}</p>
                          <p>{item.expiresAt ? `expires ${new Date(item.expiresAt).toLocaleDateString()}` : "no expiry"}</p>
                          <p>{item.id}</p>
                        </div>
                      </label>
                    ))}
                  </div>
                </article>
              ))}
            </div>
          </section>

          <section className="panel embedded-panel">
            <div className="panel-header">
              <div>
                <p className="eyebrow">Import settings</p>
                <h2>Batch import review</h2>
                <p className="section-copy">These settings will be applied to every selected secret imported into the workspace.</p>
              </div>
            </div>
            <div className="keyvault-selection-summary">
              <div className="detail-section">
                <dt>Selected secrets</dt>
                <dd>{selectedItems.length}</dd>
              </div>
              <div className="detail-section">
                <dt>Vault sources</dt>
                <dd>{selectedVaultCount}</dd>
              </div>
              <div className="detail-section wide">
                <dt>Selection preview</dt>
                {selectedItems.length === 0 ? (
                  <dd className="section-copy">Choose at least one secret on the left to review and import it here.</dd>
                ) : (
                  <div className="selected-secret-list scroll-panel">
                    {selectedItems.map((item) => (
                      <button
                        key={item.id}
                        type="button"
                        className="selected-secret-row"
                        onClick={() => toggleSecret(item.id)}
                      >
                        <div>
                          <strong>{item.name}</strong>
                          <p>{item.vaultName}</p>
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
                              onChange={() => toggleAllowedGroup(group.name)}
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
                disabled={busy || selectedItems.length === 0 || !form.owner}
                onClick={onImport}
              >
                {selectedItems.length <= 1 ? "Import selected secret" : `Import ${selectedItems.length} secrets`}
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
