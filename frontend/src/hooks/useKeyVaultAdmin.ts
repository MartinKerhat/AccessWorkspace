import { useState, type Dispatch, type SetStateAction } from "react";
import { api } from "../api/client";
import { getSelectedKeyVaultItems } from "../keyVault";
import type {
  AdminConfig,
  AdminForm,
  KeyVaultDiscoverResult,
  KeyVaultImportForm,
  KeyVaultImportItem,
  KeyVaultSyncResult,
  LaunchPayload,
  Resource,
  RevealResult,
  Session
} from "../types";

export type KeyVaultViewMode = "active" | "archived";
export type KeyVaultModalState =
  | { mode: "closed" }
  | { mode: "sources" }
  | { mode: "import" };

export const closedKeyVaultModalState: KeyVaultModalState = { mode: "closed" };

function emptyKeyVaultImportForm(): KeyVaultImportForm {
  return {
    owner: "",
    ownerTeam: "",
    environment: "",
    description: "",
    notes: "",
    allowedGroups: [],
    selectedSecretIds: []
  };
}

function summarizeKeyVaultSync(result: KeyVaultSyncResult): string {
  if (result.attemptedSources === 0) {
    return "No Key Vault source needed syncing.";
  }
  const parts = [];
  if (result.importedResources > 0) {
    parts.push(`imported ${result.importedResources} new secrets`);
  }
  parts.push(`updated ${result.updatedResources} imported secrets`);
  if (result.removedResources > 0) {
    parts.push(`removed ${result.removedResources} missing secrets`);
  }
  if (result.missingResources > 0) {
    parts.push(`${result.missingResources} needed attention`);
  }
  return parts.join(", ");
}

type UseKeyVaultAdminDeps = {
  session: Session | null;
  setBusy: (busy: boolean) => void;
  setMessage: (message: string | undefined) => void;
  adminForm: AdminForm;
  applyAdminConfigResponse: (response: AdminConfig) => void;
  loadAdminConfig: () => Promise<void>;
  loadArchivedResources: () => Promise<void>;
  loadAllResources: () => Promise<{ id: string }[]>;
  loadActivity: () => Promise<void>;
  loadAudit: () => Promise<void>;
  loadResource: (id: string) => Promise<void>;
  selectedResourceId: string | undefined;
  setSelectedResourceId: Dispatch<SetStateAction<string | undefined>>;
  setSelectedResource: Dispatch<SetStateAction<Resource | undefined>>;
  setReveal: Dispatch<SetStateAction<RevealResult | null>>;
  setLaunch: Dispatch<SetStateAction<LaunchPayload | null>>;
};

// Key Vault administration: discovery, the sources + import modals, manual /
// automatic sync, and the import flow.
export function useKeyVaultAdmin({
  session,
  setBusy,
  setMessage,
  adminForm,
  applyAdminConfigResponse,
  loadAdminConfig,
  loadArchivedResources,
  loadAllResources,
  loadActivity,
  loadAudit,
  loadResource,
  selectedResourceId,
  setSelectedResourceId,
  setSelectedResource,
  setReveal,
  setLaunch
}: UseKeyVaultAdminDeps) {
  const [keyVaultDiscoveries, setKeyVaultDiscoveries] = useState<KeyVaultDiscoverResult>({ sources: [] });
  const [keyVaultImportForm, setKeyVaultImportForm] = useState<KeyVaultImportForm>(emptyKeyVaultImportForm());
  const [keyVaultViewMode, setKeyVaultViewMode] = useState<KeyVaultViewMode>("active");
  const [selectedArchivedKeyVaultId, setSelectedArchivedKeyVaultId] = useState<string>();
  const [keyVaultSyncing, setKeyVaultSyncing] = useState(false);
  const [keyVaultModalState, setKeyVaultModalState] = useState<KeyVaultModalState>(closedKeyVaultModalState);

  async function loadKeyVaultDiscoveries() {
    const response = await api.discoverKeyVault();
    setKeyVaultDiscoveries({
      sources: (response.sources ?? []).map((source) => ({
        ...source,
        items: source.items ?? []
      }))
    });
  }

  async function handleSyncKeyVault(automatic: boolean) {
    if (!session || keyVaultSyncing) {
      return;
    }
    if (!session.user.isAdmin) {
      if (!automatic) {
        setMessage("Only admins can sync Key Vault metadata");
      }
      return;
    }
    setKeyVaultSyncing(true);
    try {
      const result = await api.syncKeyVault(automatic);
      if (session.user.isAdmin) {
        await Promise.all([loadAdminConfig(), loadArchivedResources()]);
      }
      if (result.updatedResources > 0 || result.removedResources > 0 || result.missingResources > 0 || !automatic) {
        const items = await loadAllResources();
        if (selectedResourceId && items.some((item) => item.id === selectedResourceId)) {
          await loadResource(selectedResourceId);
        } else if (selectedResourceId) {
          setSelectedResourceId(undefined);
          setSelectedResource(undefined);
          setReveal(null);
          setLaunch(null);
        }
      }
      if (!automatic && result.attemptedSources >= 0) {
        setMessage(summarizeKeyVaultSync(result));
      }
    } catch (error) {
      if (!automatic) {
        setMessage(error instanceof Error ? error.message : "Key Vault sync failed");
      }
    } finally {
      setKeyVaultSyncing(false);
    }
  }

  async function handleSaveKeyVaultSources() {
    if (!session) {
      return;
    }
    setBusy(true);
    try {
      const response = await api.updateAdminConfig(
        {
          entraTenantId: adminForm.entraTenantId,
          entraClientId: adminForm.entraClientId,
          entraAuthority: adminForm.entraAuthority,
          entraRedirectUri: adminForm.entraRedirectUri,
          entraGroupSource: adminForm.entraGroupSource,
          entraEnabled: adminForm.entraEnabled,
          rdpSigningEnabled: adminForm.rdpSigningEnabled,
          keyVaultSources: adminForm.keyVaultSources
        }
      );
      applyAdminConfigResponse(response);
      setMessage("Key Vault sources updated");
      setKeyVaultModalState(closedKeyVaultModalState);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Saving Key Vault sources failed");
    } finally {
      setBusy(false);
    }
  }

  async function openKeyVaultImport() {
    if (!session) {
      return;
    }
    setBusy(true);
    try {
      await loadKeyVaultDiscoveries();
      setKeyVaultImportForm(emptyKeyVaultImportForm());
      setKeyVaultModalState({ mode: "import" });
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Key Vault discovery failed");
    } finally {
      setBusy(false);
    }
  }

  async function handleImportKeyVaultSecret() {
    if (!session) {
      return;
    }
    setBusy(true);
    try {
      const selectedItems = getSelectedKeyVaultItems(
        keyVaultImportForm.selectedSecretIds,
        keyVaultDiscoveries.sources
      );
      const payloadItems: KeyVaultImportItem[] = selectedItems.map((item) => ({
        vaultUrl: item.vaultUrl,
        vaultName: item.vaultName,
        objectName: item.name,
        secretId: item.id,
        contentType: item.contentType,
        expiresAt: item.expiresAt,
        enabled: item.enabled
      }));
      const response = await api.importKeyVaultSecrets(
        {
          ...keyVaultImportForm,
          items: payloadItems
        }
      );
      const createdItems = response.items ?? [];
      setMessage(createdItems.length === 1 ? "Key Vault secret imported" : `${createdItems.length} Key Vault secrets imported`);
      await loadAllResources();
      await loadActivity();
      if (session.capabilities.canViewAudit) {
        await loadAudit();
      }
      if (createdItems[0]) {
        setSelectedResourceId(createdItems[0].id);
        await loadResource(createdItems[0].id);
      }
      setKeyVaultModalState(closedKeyVaultModalState);
      setKeyVaultImportForm(emptyKeyVaultImportForm());
      window.location.hash = "#keyvault";
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Importing Key Vault secret failed");
    } finally {
      setBusy(false);
    }
  }

  // Sign-out reset: mirrors exactly what App.signOut used to do inline.
  function reset() {
    setKeyVaultDiscoveries({ sources: [] });
    setKeyVaultImportForm(emptyKeyVaultImportForm());
    setKeyVaultViewMode("active");
    setSelectedArchivedKeyVaultId(undefined);
    setKeyVaultSyncing(false);
    setKeyVaultModalState(closedKeyVaultModalState);
  }

  return {
    keyVaultDiscoveries,
    loadKeyVaultDiscoveries,
    keyVaultImportForm,
    setKeyVaultImportForm,
    keyVaultViewMode,
    setKeyVaultViewMode,
    selectedArchivedKeyVaultId,
    setSelectedArchivedKeyVaultId,
    keyVaultSyncing,
    keyVaultModalState,
    setKeyVaultModalState,
    handleSyncKeyVault,
    handleSaveKeyVaultSources,
    openKeyVaultImport,
    handleImportKeyVaultSecret,
    reset
  };
}
