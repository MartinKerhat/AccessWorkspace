import type { ArchivedResourceSummary, ResourceSummary } from "./types";

export type Filters = { q: string; target: string };

export const defaultFilters: Filters = { q: "", target: "" };

// Free-text catalog search: the query matches ownership scope ("personal"/
// "shared" prefix) or any descriptive/target field; the target filter matches
// location-ish fields only.
export function filterCatalogItems(items: ResourceSummary[], filters: Filters): ResourceSummary[] {
  return items.filter((item) => {
    const query = filters.q.trim().toLowerCase();
    const target = filters.target.trim().toLowerCase();
    const ownershipScope = item.personal ? "personal" : "shared";

    const matchesQuery =
      query === "" ||
      ownershipScope.startsWith(query) ||
      item.name.toLowerCase().includes(query) ||
      item.description.toLowerCase().includes(query) ||
      item.owner.toLowerCase().includes(query) ||
      item.ownerTeam.toLowerCase().includes(query) ||
      item.folderPath.toLowerCase().includes(query) ||
      item.targetHost.toLowerCase().includes(query) ||
      item.targetUrl.toLowerCase().includes(query) ||
      item.targetSystem.toLowerCase().includes(query) ||
      item.username.toLowerCase().includes(query) ||
      item.vaultName.toLowerCase().includes(query) ||
      item.objectName.toLowerCase().includes(query) ||
      item.provider.toLowerCase().includes(query) ||
      item.applicationId.toLowerCase().includes(query);

    const matchesTarget =
      target === "" ||
      item.folderPath.toLowerCase().includes(target) ||
      item.targetHost.toLowerCase().includes(target) ||
      item.targetUrl.toLowerCase().includes(target) ||
      item.targetSystem.toLowerCase().includes(target) ||
      item.vaultName.toLowerCase().includes(target);

    return matchesQuery && matchesTarget;
  });
}

export function filterArchivedKeyVaultItems(
  items: ArchivedResourceSummary[],
  filters: Filters
): ArchivedResourceSummary[] {
  return items.filter((item) => {
    const query = filters.q.trim().toLowerCase();
    const target = filters.target.trim().toLowerCase();

    const matchesQuery =
      query === "" ||
      item.name.toLowerCase().includes(query) ||
      item.description.toLowerCase().includes(query) ||
      item.owner.toLowerCase().includes(query) ||
      item.ownerTeam.toLowerCase().includes(query) ||
      item.vaultName.toLowerCase().includes(query) ||
      item.objectName.toLowerCase().includes(query) ||
      item.archivedReason.toLowerCase().includes(query) ||
      item.archivedBy.toLowerCase().includes(query);

    const matchesTarget =
      target === "" ||
      item.vaultName.toLowerCase().includes(target) ||
      item.objectName.toLowerCase().includes(target);

    return matchesQuery && matchesTarget;
  });
}
