import { useEffect, useMemo, useState } from "react";
import type { ResourceSummary } from "../types";
import { resourceTypeLabel } from "../resourceMeta";
import { categoryDescription, categoryLabel, type WorkspaceCategory } from "../workspaceCategories";

type Filters = {
  q: string;
  target: string;
};

type Props = {
  category: WorkspaceCategory;
  filters: Filters;
  items: ResourceSummary[];
  selectedId?: string;
  canCreate?: boolean;
  createLabel?: string;
  secondaryActionLabel?: string;
  onFilterChange: (next: Filters) => void;
  onSelect: (id: string) => void;
  onCreate?: () => void;
  onSecondaryAction?: () => void;
};

type FolderNode = {
  key: string;
  label: string;
  itemCount: number;
  children: FolderNode[];
};

const allFoldersKey = "__all__";
const ungroupedFolderKey = "__ungrouped__";

function resourceDescription(item: ResourceSummary) {
  if (item.type === "key_vault_secret" && item.description.startsWith("Imported from Azure Key Vault")) {
    return "";
  }
  return item.description;
}

function isConnectionCategory(category: WorkspaceCategory) {
  return category === "connections";
}

function folderParts(path: string) {
  return path
    .split("/")
    .map((part) => part.trim())
    .filter(Boolean);
}

function buildFolderTree(items: ResourceSummary[]): FolderNode[] {
  type MutableNode = FolderNode & { childMap: Map<string, MutableNode> };

  const root = new Map<string, MutableNode>();

  function toOutput(node: MutableNode): FolderNode {
    return {
      key: node.key,
      label: node.label,
      itemCount: node.itemCount,
      children: Array.from(node.childMap.values())
        .sort((left, right) => left.label.localeCompare(right.label))
        .map(toOutput)
    };
  }

  for (const item of items) {
    const parts = folderParts(item.folderPath);
    if (parts.length === 0) {
      const existing = root.get(ungroupedFolderKey) ?? {
        key: ungroupedFolderKey,
        label: "Ungrouped",
        itemCount: 0,
        children: [],
        childMap: new Map<string, MutableNode>()
      };
      existing.itemCount += 1;
      root.set(ungroupedFolderKey, existing);
      continue;
    }

    let currentLevel = root;
    let currentPath = "";
    for (const part of parts) {
      currentPath = currentPath ? `${currentPath}/${part}` : part;
      const existing = currentLevel.get(currentPath) ?? {
        key: currentPath,
        label: part,
        itemCount: 0,
        children: [],
        childMap: new Map<string, MutableNode>()
      };
      existing.itemCount += 1;
      currentLevel.set(currentPath, existing);
      currentLevel = existing.childMap;
    }
  }

  return Array.from(root.values())
    .sort((left, right) => {
      if (left.key === ungroupedFolderKey) {
        return 1;
      }
      if (right.key === ungroupedFolderKey) {
        return -1;
      }
      return left.label.localeCompare(right.label);
    })
    .map(toOutput);
}

function collectFolderKeys(nodes: FolderNode[]): string[] {
  const keys: string[] = [];
  for (const node of nodes) {
    keys.push(node.key);
    keys.push(...collectFolderKeys(node.children));
  }
  return keys;
}

function matchesFolder(item: ResourceSummary, selectedFolder: string) {
  if (selectedFolder === allFoldersKey) {
    return true;
  }
  if (selectedFolder === ungroupedFolderKey) {
    return item.folderPath.trim() === "";
  }
  return item.folderPath === selectedFolder || item.folderPath.startsWith(`${selectedFolder}/`);
}

function renderResourceCard(
  category: WorkspaceCategory,
  item: ResourceSummary,
  selectedId: string | undefined,
  onSelect: (id: string) => void
) {
  const connectionCard = isConnectionCategory(category);

  return (
    <button
      key={item.id}
      className={`resource-card ${selectedId === item.id ? "active" : ""}`}
      onClick={() => onSelect(item.id)}
    >
      <div className="resource-card-top">
        <span className={`resource-type ${item.type}`}>{resourceTypeLabel(item.type)}</span>
        <div className="resource-actions-mini">
          {category === "passwords" ? <span>{item.personal ? "Personal" : "Shared"}</span> : null}
          {item.status && item.status !== "active" ? <span>{item.status}</span> : null}
          {item.launchAllowed ? <span>{connectionCard ? "Connect" : "Launch"}</span> : null}
        </div>
      </div>
      <h3>{item.name}</h3>
      {resourceDescription(item) ? <p>{resourceDescription(item)}</p> : null}
      <div className="meta-row">
        <span>{item.targetHost || item.targetUrl || item.targetSystem || item.objectName || item.applicationId || "No target set"}</span>
        <span>{item.owner || "No owner"}</span>
      </div>
      <div className="meta-row">
        <span>{item.folderPath || item.vaultName || item.provider || item.environment || item.status || "\u00a0"}</span>
        <span>{item.connectionDomain || item.username || item.objectName || item.credentialExpiresAt?.slice(0, 10) || "\u00a0"}</span>
      </div>
    </button>
  );
}

type FolderTreeProps = {
  nodes: FolderNode[];
  expandedFolders: string[];
  selectedFolder: string;
  totalItems: number;
  onToggle: (key: string) => void;
  onSelectFolder: (key: string) => void;
};

function FolderTree({ nodes, expandedFolders, selectedFolder, totalItems, onToggle, onSelectFolder }: FolderTreeProps) {
  function renderNode(node: FolderNode, depth: number) {
    const expanded = expandedFolders.includes(node.key);
    const hasChildren = node.children.length > 0;

    return (
      <div key={node.key} className="folder-tree-node">
        <div
          className={`folder-tree-row ${selectedFolder === node.key ? "active" : ""} ${hasChildren ? "branch" : "leaf"}`}
          style={{ paddingLeft: `${depth * 18 + 10}px` }}
        >
          <div className="folder-tree-item-main">
            {hasChildren ? (
              <button
                type="button"
                className="folder-tree-chevron-button"
                onClick={() => onToggle(node.key)}
                aria-label={expanded ? `Collapse ${node.label}` : `Expand ${node.label}`}
                aria-expanded={expanded}
              >
                <span className="folder-tree-chevron">{expanded ? "▾" : "▸"}</span>
              </button>
            ) : (
              <span className="folder-tree-leaf-marker" aria-hidden="true">
                ·
              </span>
            )}
            <button
              type="button"
              className="folder-tree-item"
              onClick={() => onSelectFolder(node.key)}
            >
              <span className="folder-tree-label">{node.label}</span>
              <span className="folder-tree-count">{node.itemCount}</span>
            </button>
          </div>
        </div>
        {hasChildren && expanded ? node.children.map((child) => renderNode(child, depth + 1)) : null}
      </div>
    );
  }

  return (
    <div className="folder-tree-panel">
      <div className="resource-section-header">
        <div>
          <p className="eyebrow">Folders</p>
          <h3>Connection tree</h3>
        </div>
        <span className="muted">{nodes.length} roots</span>
      </div>
      <div className="folder-tree-list">
        <button
          type="button"
          className={`folder-tree-all ${selectedFolder === allFoldersKey ? "active" : ""}`}
          onClick={() => onSelectFolder(allFoldersKey)}
        >
          <span>All connections</span>
          <span className="folder-tree-count">{totalItems}</span>
        </button>
        {nodes.map((node) => renderNode(node, 0))}
      </div>
    </div>
  );
}

export function CatalogPage({
  category,
  filters,
  items,
  selectedId,
  canCreate,
  createLabel,
  secondaryActionLabel,
  onFilterChange,
  onSelect,
  onCreate,
  onSecondaryAction
}: Props) {
  const connectionCategory = isConnectionCategory(category);
  const folderNodes = useMemo(() => (connectionCategory ? buildFolderTree(items) : []), [connectionCategory, items]);
  const [selectedFolder, setSelectedFolder] = useState<string>(allFoldersKey);
  const [expandedFolders, setExpandedFolders] = useState<string[]>([]);

  useEffect(() => {
    if (!connectionCategory) {
      setSelectedFolder(allFoldersKey);
      setExpandedFolders([]);
      return;
    }

    const availableKeys = new Set([allFoldersKey, ...collectFolderKeys(folderNodes)]);
    setExpandedFolders((current) => {
      const next = current.filter((key) => availableKeys.has(key));
      if (next.length > 0) {
        return next;
      }
      return folderNodes.map((node) => node.key);
    });
    setSelectedFolder((current) => (availableKeys.has(current) ? current : allFoldersKey));
  }, [category, connectionCategory, folderNodes]);

  const visibleItems = connectionCategory ? items.filter((item) => matchesFolder(item, selectedFolder)) : items;

  return (
    <section className="catalog-layout">
      <div className="panel">
        <div className="panel-header">
          <div>
            <p className="eyebrow">Category</p>
            <h2>{categoryLabel(category)}</h2>
            <p className="section-copy">{categoryDescription(category)}</p>
          </div>
          <div className="panel-header-actions">
            <span className="muted">{items.length} results</span>
            {secondaryActionLabel && onSecondaryAction ? (
              <button className="button ghost" onClick={onSecondaryAction}>
                {secondaryActionLabel}
              </button>
            ) : null}
            {canCreate && onCreate ? (
              <button className="button primary" onClick={onCreate}>
                {createLabel ?? "Create"}
              </button>
            ) : null}
          </div>
        </div>

        <div className="filter-grid">
          <input
            className={connectionCategory ? "wide-filter-input" : undefined}
            placeholder={
              connectionCategory
                ? "Search name, host, folder, owner"
                : category === "passwords"
                  ? "Search name, target, owner, personal/shared"
                  : "Search name, target, owner, provider"
            }
            value={filters.q}
            onChange={(event) =>
              onFilterChange(connectionCategory ? { q: event.target.value, target: "" } : { ...filters, q: event.target.value })
            }
          />
          {!connectionCategory ? (
            <input
              placeholder="Target or host"
              value={filters.target}
              onChange={(event) => onFilterChange({ ...filters, target: event.target.value })}
            />
          ) : null}
        </div>
      </div>

      <div className={`resource-list-panel ${connectionCategory ? "connections-browser-panel" : ""}`}>
        {connectionCategory ? (
          <FolderTree
            nodes={folderNodes}
            expandedFolders={expandedFolders}
            selectedFolder={selectedFolder}
            totalItems={items.length}
            onToggle={(key) =>
              setExpandedFolders((current) =>
                current.includes(key) ? current.filter((item) => item !== key) : [...current, key]
              )
            }
            onSelectFolder={setSelectedFolder}
          />
        ) : null}
        <div className="connections-results-panel">
          <div className="resource-list-header">
            <p className="eyebrow">Results</p>
            <span className="muted">{visibleItems.length} matching resources</span>
          </div>
          {visibleItems.length === 0 ? (
            <div className="panel empty-state-panel">
              <p className="eyebrow">No results</p>
              <h3>No resources match the current filters.</h3>
              <p className="section-copy">Try clearing one of the filters or broadening the search terms.</p>
            </div>
          ) : null}
          <div className="resource-section-list">
            {visibleItems.map((item) => renderResourceCard(category, item, selectedId, onSelect))}
          </div>
        </div>
      </div>
    </section>
  );
}
