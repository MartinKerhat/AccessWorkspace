import type { ArchivedResourceSummary } from "../types";
import { resourceTypeLabel } from "../resourceMeta";

type Filters = {
  q: string;
  target: string;
};

type Props = {
  filters: Filters;
  items: ArchivedResourceSummary[];
  selectedId?: string;
  loading?: boolean;
  onFilterChange: (next: Filters) => void;
  onSelect: (id: string) => void;
  onRestore: (item: ArchivedResourceSummary) => void;
};

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

function archivedReasonLabel(reason: string) {
  switch (reason) {
    case "removed_from_app":
      return "Removed from app";
    case "key_vault_secret_not_found":
      return "Missing in Azure Key Vault during sync";
    case "":
      return "Archived";
    default:
      return reason.split("_").join(" ");
  }
}

function archivedSummaryLine(item: ArchivedResourceSummary) {
  const parts = [item.vaultName, item.objectName].filter(Boolean);
  return parts.join(" / ");
}

export function ArchivedKeyVaultPage({
  filters,
  items,
  selectedId,
  loading,
  onFilterChange,
  onSelect,
  onRestore
}: Props) {
  const selectedItem = items.find((item) => item.id === selectedId);

  return (
    <div className="workspace-grid">
      <section className="catalog-layout">
        <div className="panel">
          <div className="panel-header">
            <div>
              <p className="eyebrow">Category</p>
              <h2>Key Vault</h2>
              <p className="section-copy">
                Review archived Key Vault objects, see why they left the active catalog, and restore them when they should come back.
              </p>
            </div>
            <div className="panel-header-actions">
              <span className="muted">{items.length} archived</span>
            </div>
          </div>

          <div className="filter-grid">
            <input
              placeholder="Search name, object, owner, reason"
              value={filters.q}
              onChange={(event) => onFilterChange({ ...filters, q: event.target.value })}
            />
            <input
              placeholder="Vault or object"
              value={filters.target}
              onChange={(event) => onFilterChange({ ...filters, target: event.target.value })}
            />
          </div>
        </div>

        <div className="resource-list-panel">
          <div className="resource-list-header">
            <p className="eyebrow">Archived results</p>
            <span className="muted">{items.length} matching resources</span>
          </div>
          {items.length === 0 ? (
            <div className="panel empty-state-panel">
              <p className="eyebrow">No archived objects</p>
              <h3>No archived Key Vault resources match the current filters.</h3>
              <p className="section-copy">Try clearing a filter or switch back to the active Key Vault view.</p>
            </div>
          ) : null}
          {items.map((item) => (
            <button
              key={item.id}
              className={`resource-card ${selectedId === item.id ? "active" : ""}`}
              onClick={() => onSelect(item.id)}
            >
              <div className="resource-card-top">
                <span className={`resource-type ${item.type}`}>{resourceTypeLabel(item.type)}</span>
                <div className="resource-actions-mini">
                  <span>{archivedReasonLabel(item.archivedReason)}</span>
                </div>
              </div>
              <h3>{item.name}</h3>
              <p>{archivedSummaryLine(item) || "No Key Vault object identity recorded."}</p>
              <div className="meta-row">
                <span>{item.owner || "No owner"}</span>
                <span>{item.archivedBy || "Unknown"}</span>
              </div>
              <div className="meta-row">
                <span>{item.vaultName || "No vault"}</span>
                <span>{formatDateTime(item.archivedAt)}</span>
              </div>
            </button>
          ))}
        </div>
      </section>

      <section className="panel detail-panel">
        {!selectedItem ? (
          <>
            <p className="eyebrow">Archived details</p>
            <h2>Select an archived Key Vault object</h2>
            <p className="section-copy">Pick an archived object from the list to review why it was archived and restore it.</p>
          </>
        ) : (
          <>
            <div className="panel-header">
              <div className="detail-header-copy">
                <p className="eyebrow">Archived details</p>
                <h2>{selectedItem.name}</h2>
                <p className="detail-description">{archivedReasonLabel(selectedItem.archivedReason)}</p>
              </div>
              <div className="detail-header-actions">
                <span className={`resource-type ${selectedItem.type}`}>{resourceTypeLabel(selectedItem.type)}</span>
                <button className="button ghost" disabled={loading} onClick={() => onRestore(selectedItem)}>
                  Restore
                </button>
              </div>
            </div>

            {selectedItem.description ? (
              <div className="detail-section">
                <p className="eyebrow">Purpose</p>
                <p className="detail-description">{selectedItem.description}</p>
              </div>
            ) : null}

            <dl className="detail-grid">
              <div>
                <dt>Archived</dt>
                <dd>{formatDateTime(selectedItem.archivedAt)}</dd>
              </div>
              <div>
                <dt>Reason</dt>
                <dd>{archivedReasonLabel(selectedItem.archivedReason)}</dd>
              </div>
              <div>
                <dt>Archived by</dt>
                <dd>{selectedItem.archivedBy || "Unknown"}</dd>
              </div>
              <div>
                <dt>Status</dt>
                <dd>{selectedItem.status || "n/a"}</dd>
              </div>
              <div>
                <dt>Vault</dt>
                <dd>{selectedItem.vaultName || "n/a"}</dd>
              </div>
              <div>
                <dt>Object name</dt>
                <dd>{selectedItem.objectName || "n/a"}</dd>
              </div>
              <div>
                <dt>Owner</dt>
                <dd>{selectedItem.owner || "n/a"}</dd>
              </div>
              <div>
                <dt>Owner team</dt>
                <dd>{selectedItem.ownerTeam || "n/a"}</dd>
              </div>
              <div>
                <dt>Environment</dt>
                <dd>{selectedItem.environment || "n/a"}</dd>
              </div>
              <div>
                <dt>Allowed groups</dt>
                <dd>{selectedItem.allowedGroups.length > 0 ? selectedItem.allowedGroups.join(", ") : "Everyone"}</dd>
              </div>
            </dl>
          </>
        )}
      </section>
    </div>
  );
}
