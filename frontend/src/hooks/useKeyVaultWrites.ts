import { useState } from "react";
import { api } from "../api/client";
import type {
  KeyVaultCreateSecretInput,
  KeyVaultSecretStateInput,
  KeyVaultSecretValueInput,
  KeyVaultSecretVersion,
  Resource,
  ResourceSummary,
  RevealResult,
  Session,
  WritableKeyVaultSource
} from "../types";

// Writes to Azure Key Vault from the workspace: a new secret (keyvault.write),
// a new value for an existing one (keyvault.write) and enabled / active window
// of one version (keyvault.edit). The value only passes through the open modal
// and one API call; nothing here keeps it.
export type KeyVaultWriteModalState =
  | { mode: "closed" }
  | { mode: "create" }
  | { mode: "value"; resource: Resource }
  | { mode: "state"; resource: Resource; version: KeyVaultSecretVersion };

type UseKeyVaultWritesDeps = {
  session: Session | null;
  setBusy: (busy: boolean) => void;
  setMessage: (message: string | undefined) => void;
  loadAllResources: () => Promise<ResourceSummary[]>;
  loadResource: (id: string) => Promise<void>;
  loadActivity: () => Promise<void>;
  setSelectedResourceId: (id: string) => void;
  // Shows a revealed value in the app's reveal modal.
  showReveal: (result: RevealResult) => void;
};

function keyVaultReference(resource: Resource) {
  return (resource.secret.reference || resource.sourceObjectId || "").toLowerCase();
}

// …/secrets/{name}/{version} — the object is tied to one version.
function isPinned(resource?: Resource) {
  const [, path = ""] = (resource ? keyVaultReference(resource) : "").split("/secrets/");
  return path.replace(/\/+$/, "").split("/").length > 1;
}

export function useKeyVaultWrites({
  session,
  setBusy,
  setMessage,
  loadAllResources,
  loadResource,
  loadActivity,
  setSelectedResourceId,
  showReveal
}: UseKeyVaultWritesDeps) {
  const [writableSources, setWritableSources] = useState<WritableKeyVaultSource[]>([]);
  const [modalState, setModalState] = useState<KeyVaultWriteModalState>({ mode: "closed" });

  const keyVaultCapabilities = session?.capabilities.categories.keyvault;
  const isAdmin = Boolean(session?.user.isAdmin);
  const canCreate = isAdmin || Boolean(keyVaultCapabilities?.create);
  const canWriteValue = isAdmin || Boolean(keyVaultCapabilities?.write);
  const canChangeState = isAdmin || Boolean(keyVaultCapabilities?.edit);

  // Read whenever the Key Vault category opens: an admin may have just
  // opened or closed a vault for writes.
  async function loadWritableSources() {
    if (!session || !(canCreate || canWriteValue || canChangeState)) {
      setWritableSources([]);
      return [];
    }
    try {
      const response = await api.listWritableKeyVaultSources();
      setWritableSources(response.items);
      return response.items;
    } catch {
      setWritableSources([]);
      return [];
    }
  }

  function isWritable(resource?: Resource) {
    if (!resource || resource.type !== "key_vault_secret" || resource.sourceKind !== "azure_key_vault") {
      return false;
    }
    const reference = keyVaultReference(resource);
    return writableSources.some((source) => reference.startsWith(`${source.vaultUrl.toLowerCase().replace(/\/+$/, "")}/`));
  }

  async function openCreate() {
    setMessage(undefined);
    await loadWritableSources();
    setModalState({ mode: "create" });
  }

  async function afterWrite(resource: Resource, message: string) {
    setModalState({ mode: "closed" });
    setMessage(message);
    await loadAllResources();
    setSelectedResourceId(resource.id);
    await loadResource(resource.id);
    await loadActivity();
  }

  async function run(action: () => Promise<Resource>, success: string, failure: string) {
    setBusy(true);
    try {
      const resource = await action();
      await afterWrite(resource, success);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : failure);
    } finally {
      setBusy(false);
    }
  }

  return {
    writableSources,
    modalState,
    closeModal: () => setModalState({ mode: "closed" }),
    loadWritableSources,
    canCreate,
    openCreate,
    // Buttons on the detail page appear only when they can work: the right,
    // and a vault an admin opened for writes.
    // A pinned object would never see a new version, and the API refuses it.
    canSetValue: (resource?: Resource) => canWriteValue && isWritable(resource) && !isPinned(resource),
    canEditState: (resource?: Resource) => canChangeState && isWritable(resource),
    openSetValue: (resource: Resource) => {
      setMessage(undefined);
      setModalState({ mode: "value", resource });
    },
    openState: (resource: Resource, version: KeyVaultSecretVersion) => {
      setMessage(undefined);
      setModalState({ mode: "state", resource, version });
    },
    createSecret: (input: KeyVaultCreateSecretInput) =>
      run(() => api.createKeyVaultSecret(input), "Secret created in Azure and added to the catalog", "Creating the Key Vault secret failed"),
    setValue: (resource: Resource, input: KeyVaultSecretValueInput) =>
      run(() => api.setKeyVaultSecretValue(resource.id, input), "New value saved in Azure", "Saving the new value failed"),
    updateState: (resource: Resource, version: string, input: KeyVaultSecretStateInput) =>
      run(
        () => api.updateKeyVaultSecretVersionState(resource.id, version, input),
        `Version ${version.slice(0, 8)} saved in Azure`,
        "Saving the version state failed"
      ),
    revealVersion: async (resource: Resource, version: string) => {
      setBusy(true);
      try {
        showReveal(await api.revealKeyVaultSecretVersion(resource.id, version));
        await loadActivity();
      } catch (error) {
        setMessage(error instanceof Error ? error.message : "Reveal failed");
      } finally {
        setBusy(false);
      }
    }
  };
}
