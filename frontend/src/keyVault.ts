import type { KeyVaultDiscoverResult, KeyVaultDiscoveredSecret, KeyVaultSource, ResourceSummary } from "./types";

export function getSelectedKeyVaultItems(
  selectedSecretIds: string[],
  sources: KeyVaultDiscoverResult["sources"]
): KeyVaultDiscoveredSecret[] {
  const selectedIds = new Set(selectedSecretIds);
  return sources.flatMap((source) => source.items).filter((item) => selectedIds.has(item.id));
}

export function emptyKeyVaultSource(): KeyVaultSource {
  return {
    name: "",
    vaultUrl: "",
    syncEnabled: true,
    syncIntervalMinutes: 60,
    autoImportEnabled: false,
    defaultOwner: "",
    defaultOwnerTeam: "",
    defaultEnvironment: "",
    defaultDescription: "",
    defaultNotes: "",
    defaultAllowedGroups: [],
    lastSyncedAt: undefined,
    lastSyncStatus: "",
    lastSyncError: "",
    lastSyncSummary: ""
  };
}

// Identity of a discovered secret from the fields the resource summary
// exposes. The backend dedupes on the full secret id, which the summary does
// not carry; vault name + object name is the closest stable pair and is what
// both import paths store on the resource.
export function keyVaultSecretKey(vaultName: string, objectName: string): string {
  return `${vaultName.trim().toLowerCase()}/${objectName.trim().toLowerCase()}`;
}

export function importedKeyVaultSecretKeys(resources: ResourceSummary[]): Set<string> {
  return new Set(
    resources
      .filter((item) => item.type === "key_vault_secret" && item.sourceKind === "azure_key_vault")
      .filter((item) => item.vaultName && item.objectName)
      .map((item) => keyVaultSecretKey(item.vaultName, item.objectName))
  );
}
