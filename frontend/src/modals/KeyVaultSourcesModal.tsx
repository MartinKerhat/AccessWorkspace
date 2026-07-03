import { useState } from "react";
import type { KeyVaultSource, LocalGroup, UserSummary } from "../types";
import { emptyKeyVaultSource } from "../keyVault";

type PickerState = { index: number; kind: "owner" | "ownerTeam" | "group" } | null;

type Props = {
  sources: KeyVaultSource[];
  setSources: (updater: (prev: KeyVaultSource[]) => KeyVaultSource[]) => void;
  knownUsers: UserSummary[];
  localGroups: LocalGroup[];
  busy: boolean;
  onSave: () => void;
  onClose: () => void;
};

export function KeyVaultSourcesModal({ sources, setSources, knownUsers, localGroups, busy, onSave, onClose }: Props) {
  const [picker, setPicker] = useState<PickerState>(null);
  const [ownerSearch, setOwnerSearch] = useState("");
  const [ownerTeamSearch, setOwnerTeamSearch] = useState("");
  const [groupSearch, setGroupSearch] = useState("");

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

  function closePickers() {
    setPicker(null);
    setOwnerSearch("");
    setOwnerTeamSearch("");
    setGroupSearch("");
  }

  function togglePicker(index: number, kind: NonNullable<PickerState>["kind"]) {
    setPicker((current) => (current?.index === index && current.kind === kind ? null : { index, kind }));
    if (kind !== "owner") {
      setOwnerSearch("");
    }
    if (kind !== "ownerTeam") {
      setOwnerTeamSearch("");
    }
    if (kind !== "group") {
      setGroupSearch("");
    }
  }

  function updateSource(index: number, patch: Partial<KeyVaultSource>) {
    setSources((current) => current.map((item, itemIndex) => (itemIndex === index ? { ...item, ...patch } : item)));
  }

  function toggleAllowedGroup(index: number, group: string) {
    setSources((current) =>
      current.map((item, itemIndex) => {
        if (itemIndex !== index) {
          return item;
        }
        return {
          ...item,
          defaultAllowedGroups: item.defaultAllowedGroups.includes(group)
            ? item.defaultAllowedGroups.filter((currentGroup) => currentGroup !== group)
            : [...item.defaultAllowedGroups, group]
        };
      })
    );
  }

  function addSource() {
    closePickers();
    setSources((current) => [...current, emptyKeyVaultSource()]);
  }

  function removeSource(index: number) {
    closePickers();
    setSources((current) => current.filter((_, itemIndex) => itemIndex !== index));
  }

  return (
    <div className="modal-scrim" onClick={onClose}>
      <div className="modal-card" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <p className="eyebrow">Secret sources</p>
            <h2>Edit Key Vault sources</h2>
          </div>
          <button className="button ghost" onClick={onClose}>
            Close
          </button>
        </div>
        <div className="group-list">
          {sources.map((source, index) => (
            <div key={`${source.vaultUrl}-${index}`} className="form-grid source-grid">
              <label>
                <span>Name</span>
                <input
                  value={source.name}
                  onChange={(event) => updateSource(index, { name: event.target.value })}
                  placeholder="finance-vault"
                />
              </label>
              <label>
                <span>Vault URL</span>
                <input
                  value={source.vaultUrl}
                  onChange={(event) => updateSource(index, { vaultUrl: event.target.value })}
                  placeholder="https://finance-vault.vault.azure.net"
                />
              </label>
              <label>
                <span>Sync interval (minutes)</span>
                <input
                  type="number"
                  min={5}
                  step={5}
                  value={source.syncIntervalMinutes}
                  onChange={(event) =>
                    updateSource(index, {
                      syncIntervalMinutes: Number.parseInt(event.target.value || "60", 10) || 60
                    })
                  }
                />
              </label>
              <label className="checkbox">
                <input
                  type="checkbox"
                  checked={source.syncEnabled}
                  onChange={(event) => updateSource(index, { syncEnabled: event.target.checked })}
                />
                <span>Automatic sync</span>
              </label>
              <label className="checkbox">
                <input
                  type="checkbox"
                  checked={source.autoImportEnabled}
                  onChange={(event) => updateSource(index, { autoImportEnabled: event.target.checked })}
                />
                <span>Auto import</span>
              </label>
              <label>
                <span>Default owner</span>
                <div className="picker-shell">
                  <button type="button" className="single-picker-trigger" onClick={() => togglePicker(index, "owner")}>
                    <span>{source.defaultOwner || "Select default owner"}</span>
                    <span>{picker?.index === index && picker.kind === "owner" ? "Close" : "Select"}</span>
                  </button>
                  {picker?.index === index && picker.kind === "owner" ? (
                    <div className="group-picker-dropdown">
                      <input
                        className="picker-search-input"
                        value={ownerSearch}
                        onChange={(event) => setOwnerSearch(event.target.value)}
                        placeholder="Search users"
                      />
                      <div className="picker-option-list">
                        {source.defaultOwner ? (
                          <button
                            type="button"
                            className="picker-option"
                            onClick={() => {
                              updateSource(index, { defaultOwner: "" });
                              closePickers();
                            }}
                          >
                            <strong>Clear selection</strong>
                          </button>
                        ) : null}
                        {filteredOwners.map((owner) => (
                          <button
                            key={owner.id}
                            type="button"
                            className={`picker-option ${source.defaultOwner === owner.name ? "active" : ""}`}
                            onClick={() => {
                              updateSource(index, { defaultOwner: owner.name });
                              closePickers();
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
                {source.autoImportEnabled && !source.defaultOwner ? (
                  <span className="selection-hint">Auto import needs a default owner.</span>
                ) : null}
              </label>
              <label>
                <span>Default owner team</span>
                <div className="picker-shell">
                  <button type="button" className="single-picker-trigger" onClick={() => togglePicker(index, "ownerTeam")}>
                    <span>{source.defaultOwnerTeam || "Select default owner team"}</span>
                    <span>{picker?.index === index && picker.kind === "ownerTeam" ? "Close" : "Select"}</span>
                  </button>
                  {picker?.index === index && picker.kind === "ownerTeam" ? (
                    <div className="group-picker-dropdown">
                      <input
                        className="picker-search-input"
                        value={ownerTeamSearch}
                        onChange={(event) => setOwnerTeamSearch(event.target.value)}
                        placeholder="Search local groups"
                      />
                      <div className="picker-option-list">
                        {source.defaultOwnerTeam ? (
                          <button
                            type="button"
                            className="picker-option"
                            onClick={() => {
                              updateSource(index, { defaultOwnerTeam: "" });
                              closePickers();
                            }}
                          >
                            <strong>Clear selection</strong>
                          </button>
                        ) : null}
                        {filteredOwnerTeams.map((group) => (
                          <button
                            key={group.name}
                            type="button"
                            className={`picker-option ${source.defaultOwnerTeam === group.name ? "active" : ""}`}
                            onClick={() => {
                              updateSource(index, { defaultOwnerTeam: group.name });
                              closePickers();
                            }}
                          >
                            <strong>{group.name}</strong>
                          </button>
                        ))}
                        {filteredOwnerTeams.length === 0 ? (
                          <span className="selection-hint">
                            {localGroups.length === 0 ? "No local groups available." : "No groups match the current search."}
                          </span>
                        ) : null}
                      </div>
                    </div>
                  ) : null}
                </div>
              </label>
              <label>
                <span>Default environment</span>
                <input
                  value={source.defaultEnvironment}
                  onChange={(event) => updateSource(index, { defaultEnvironment: event.target.value })}
                  placeholder="prod"
                />
              </label>
              <label className="wide">
                <span>Default description</span>
                <textarea
                  rows={2}
                  value={source.defaultDescription}
                  onChange={(event) => updateSource(index, { defaultDescription: event.target.value })}
                />
              </label>
              <label className="wide">
                <span>Default notes</span>
                <textarea
                  rows={2}
                  value={source.defaultNotes}
                  onChange={(event) => updateSource(index, { defaultNotes: event.target.value })}
                />
              </label>
              <label className="wide">
                <span>Default allowed groups</span>
                <div className="picker-shell">
                  <button type="button" className="group-picker-trigger" onClick={() => togglePicker(index, "group")}>
                    <div className="group-picker-value">
                      {source.defaultAllowedGroups.length > 0 ? (
                        source.defaultAllowedGroups.map((group) => (
                          <span key={group} className="tag">
                            {group}
                          </span>
                        ))
                      ) : (
                        <span className="selection-hint">Everyone</span>
                      )}
                    </div>
                    <span>{picker?.index === index && picker.kind === "group" ? "Close" : "Select"}</span>
                  </button>
                  {picker?.index === index && picker.kind === "group" ? (
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
                              checked={source.defaultAllowedGroups.includes(group.name)}
                              onChange={() => toggleAllowedGroup(index, group.name)}
                            />
                            <span>{group.name}</span>
                          </label>
                        ))}
                        {filteredGroups.length === 0 ? (
                          <span className="selection-hint">
                            {localGroups.length === 0
                              ? "No local groups available. Everyone can see auto-imported resources."
                              : "No groups match the current search."}
                          </span>
                        ) : null}
                      </div>
                    </div>
                  ) : null}
                </div>
              </label>
              <div className="wide action-row">
                <button className="button ghost" onClick={() => removeSource(index)}>
                  Remove source
                </button>
              </div>
            </div>
          ))}
          <button className="button ghost" onClick={addSource}>
            Add source
          </button>
        </div>
        <div className="action-row">
          <button className="button primary" disabled={busy} onClick={onSave}>
            Save Key Vault sources
          </button>
          <button className="button ghost" onClick={onClose}>
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}
