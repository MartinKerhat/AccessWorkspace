import type { KeyVaultDiscoverResult, KeyVaultDiscoveredSecret, KeyVaultSource } from "./types";

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
