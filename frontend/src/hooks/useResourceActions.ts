import { useEffect, useState, type Dispatch, type SetStateAction } from "react";
import { api } from "../api/client";
import type {
  ArchivedResourceSummary,
  ConnectionCredentialOverride,
  LaunchPayload,
  LauncherRuntime,
  Resource,
  ResourceForm,
  ResourceSummary,
  RevealResult,
  Session
} from "../types";

// Dotted numeric version compare ("0.5.8" style); missing segments count as 0.
// A launcher newer than the published requirement is fine — only older fails.
function isVersionOlder(current: string, required: string) {
  const currentParts = current.trim().replace(/^v/, "").split(".");
  const requiredParts = required.trim().replace(/^v/, "").split(".");
  const length = Math.max(currentParts.length, requiredParts.length);
  for (let index = 0; index < length; index += 1) {
    const left = Number.parseInt(currentParts[index] ?? "0", 10) || 0;
    const right = Number.parseInt(requiredParts[index] ?? "0", 10) || 0;
    if (left !== right) {
      return left < right;
    }
  }
  return false;
}

type UseResourceActionsDeps = {
  session: Session | null;
  setBusy: (busy: boolean) => void;
  setMessage: (message: string | undefined) => void;
  selectedResourceId: string | undefined;
  setSelectedResourceId: Dispatch<SetStateAction<string | undefined>>;
  selectedResource: Resource | undefined;
  setSelectedResource: Dispatch<SetStateAction<Resource | undefined>>;
  guardVaultLocked: (error: unknown, retry: () => Promise<void>) => Promise<boolean>;
  launcherRuntime: LauncherRuntime | null;
  setLauncherRuntime: Dispatch<SetStateAction<LauncherRuntime | null>>;
  refreshLauncherStatus: (runtimeArg?: LauncherRuntime | null) => Promise<{ version: string } | null>;
  loadAllResources: () => Promise<ResourceSummary[]>;
  loadResource: (id: string) => Promise<void>;
  loadActivity: () => Promise<void>;
  loadAudit: () => Promise<void>;
  loadArchivedResources: () => Promise<void>;
  closeResourceForm: () => void;
};

// Actions on the selected resource: reveal, launch (browser + desktop
// launcher hand-off), personal connection overrides, create/update/archive/
// restore, and the selection-driven side state (password options, override).
export function useResourceActions({
  session,
  setBusy,
  setMessage,
  selectedResourceId,
  setSelectedResourceId,
  selectedResource,
  setSelectedResource,
  guardVaultLocked,
  launcherRuntime,
  setLauncherRuntime,
  refreshLauncherStatus,
  loadAllResources,
  loadResource,
  loadActivity,
  loadAudit,
  loadArchivedResources,
  closeResourceForm
}: UseResourceActionsDeps) {
  const [passwordOptions, setPasswordOptions] = useState<ResourceSummary[]>([]);
  const [connectionOverride, setConnectionOverride] = useState<ConnectionCredentialOverride | null>(null);
  const [reveal, setReveal] = useState<RevealResult | null>(null);
  const [launch, setLaunch] = useState<LaunchPayload | null>(null);
  const [revealCopyMessage, setRevealCopyMessage] = useState<string>();

  useEffect(() => {
    let cancelled = false;

    async function loadConnectionPersonalization() {
      if (!selectedResource || !session || (selectedResource.type !== "rdp" && selectedResource.type !== "ssh")) {
        setPasswordOptions([]);
        setConnectionOverride(null);
        return;
      }
      if (!session.capabilities.categories.passwords.view) {
        setPasswordOptions([]);
        setConnectionOverride(null);
        return;
      }
      try {
        const [optionsResponse, overrideResponse] = await Promise.all([
          api.listPasswordOptions(),
          api.getConnectionCredentialOverride(selectedResource.id)
        ]);
        if (cancelled) {
          return;
        }
        setPasswordOptions(optionsResponse.items);
        setConnectionOverride(overrideResponse);
      } catch (error) {
        if (!cancelled) {
          setPasswordOptions([]);
          setConnectionOverride(null);
          setMessage(error instanceof Error ? error.message : "Failed to load connection override options");
        }
      }
    }

    void loadConnectionPersonalization();
    return () => {
      cancelled = true;
    };
  }, [selectedResource, session]);

  useEffect(() => {
    if (!selectedResourceId || !session) {
      setSelectedResource(undefined);
      setPasswordOptions([]);
      setConnectionOverride(null);
      return;
    }
    void loadResource(selectedResourceId);
  }, [selectedResourceId, session]);

  useEffect(() => {
    if (!reveal?.secretValue) {
      setRevealCopyMessage(undefined);
    }
  }, [reveal]);

  async function refreshAfterSensitiveAction() {
    if (!session) {
      return;
    }
    await loadActivity();
    if (session.capabilities.canViewAudit) {
      await loadAudit();
    }
  }

  async function handleReveal() {
    if (!selectedResourceId || !session) {
      return undefined;
    }
    setBusy(true);
    try {
      const response = await api.revealResource(selectedResourceId);
      setReveal(response);
      await refreshAfterSensitiveAction();
      return response.secretValue;
    } catch (error) {
      if (await guardVaultLocked(error, () => handleReveal().then(() => undefined))) {
        return undefined;
      }
      setMessage(error instanceof Error ? error.message : "Reveal failed");
      return undefined;
    } finally {
      setBusy(false);
    }
  }

  async function handleRevealStoredPassword() {
    if (!selectedResourceId || !session) {
      return undefined;
    }
    setBusy(true);
    try {
      const response = await api.revealResource(selectedResourceId);
      await refreshAfterSensitiveAction();
      return response.secretValue;
    } catch (error) {
      if (await guardVaultLocked(error, () => handleRevealStoredPassword().then(() => undefined))) {
        return undefined;
      }
      setMessage(error instanceof Error ? error.message : "Reveal failed");
      return undefined;
    } finally {
      setBusy(false);
    }
  }

  async function handleCopyRevealSecret() {
    if (!reveal?.secretValue) {
      return;
    }
    try {
      await navigator.clipboard.writeText(reveal.secretValue);
      setRevealCopyMessage("Secret copied to clipboard");
    } catch {
      setRevealCopyMessage("Copying the secret failed");
    }
  }

  async function handleLaunch() {
    if (!selectedResourceId || !session) {
      return;
    }
    setBusy(true);
    try {
      const response = await api.launchResource(selectedResourceId);
      setLaunch(response);
      if (selectedResource?.type === "rdp" || selectedResource?.type === "ssh") {
        let runtime = launcherRuntime;
        if (!runtime) {
          runtime = await api.launcherRuntime();
          setLauncherRuntime(runtime);
        }
        const status = await refreshLauncherStatus(runtime);
        if (!status) {
          setMessage("Launcher not detected. Download and install the desktop launcher first.");
          return;
        }
        if (isVersionOlder(status.version, runtime.requiredVersion)) {
          setMessage(`Launcher ${status.version} is outdated. Install version ${runtime.requiredVersion}.`);
          return;
        }
        const launcherTicket = typeof response.metadata.launcherTicket === "string" ? response.metadata.launcherTicket : "";
        const preparedPayload: LaunchPayload = launcherTicket
          ? {
              ...response,
              metadata: {
                ...response.metadata,
                launcherResolveUrl: api.launcherTicketResolveUrl(launcherTicket)
              }
            }
          : response;
        await api.launcherLocalLaunch(runtime.launchUrl, preparedPayload);
        setMessage("Connection handed off to the desktop launcher.");
      } else if (selectedResource?.type === "web_portal") {
        if (response.url) {
          setLaunch(null);
          window.open(response.url, "_blank", "noopener,noreferrer");
          setMessage("Target opened in a new browser tab.");
        } else {
          setMessage("Launch target prepared.");
        }
      }
      await refreshAfterSensitiveAction();
    } catch (error) {
      if (await guardVaultLocked(error, () => handleLaunch())) {
        return;
      }
      setMessage(error instanceof Error ? error.message : "Launch failed");
    } finally {
      setBusy(false);
    }
  }

  async function handleSaveConnectionOverride(passwordResourceId: string) {
    if (!selectedResourceId || !session) {
      return;
    }
    setBusy(true);
    try {
      const override = await api.setConnectionCredentialOverride(selectedResourceId, passwordResourceId);
      setConnectionOverride(override);
      setMessage("Personal connection override saved.");
      if (session.capabilities.canViewAudit) {
        await loadAudit();
      }
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Saving the connection override failed");
    } finally {
      setBusy(false);
    }
  }

  async function handleClearConnectionOverride() {
    if (!selectedResourceId || !session) {
      return;
    }
    setBusy(true);
    try {
      await api.clearConnectionCredentialOverride(selectedResourceId);
      setConnectionOverride({
        connectionId: selectedResourceId,
        passwordResourceId: "",
        passwordResourceName: "",
        username: "",
        personal: false
      });
      setMessage("Personal connection override cleared.");
      if (session.capabilities.canViewAudit) {
        await loadAudit();
      }
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Clearing the connection override failed");
    } finally {
      setBusy(false);
    }
  }

  async function handleCreate(input: ResourceForm) {
    if (!session) {
      return;
    }
    setBusy(true);
    try {
      const created = await api.createResource(input);
      setMessage("Object created");
      await loadAllResources();
      setSelectedResourceId(created.id);
      await loadResource(created.id);
      if (session.capabilities.canViewAudit) {
        await loadAudit();
      }
      closeResourceForm();
    } catch (error) {
      if (await guardVaultLocked(error, () => handleCreate(input))) {
        return;
      }
      setMessage(error instanceof Error ? error.message : "Create failed");
    } finally {
      setBusy(false);
    }
  }

  async function handleUpdate(input: ResourceForm) {
    if (!selectedResourceId || !session) {
      return;
    }
    setBusy(true);
    try {
      await api.updateResource(selectedResourceId, input);
      setMessage("Object updated");
      await loadAllResources();
      await loadResource(selectedResourceId);
      if (session.capabilities.canViewAudit) {
        await loadAudit();
      }
      closeResourceForm();
    } catch (error) {
      if (await guardVaultLocked(error, () => handleUpdate(input))) {
        return;
      }
      setMessage(error instanceof Error ? error.message : "Update failed");
    } finally {
      setBusy(false);
    }
  }

  async function handleArchive() {
    if (!selectedResourceId || !selectedResource || !session) {
      return;
    }
    // Ownership alone allows removal (mirrors the backend rule); admins may
    // remove shared objects they do not own.
    const isOwner = selectedResource.ownerUserId === session.user.id;
    if (!isOwner && !(session.user.isAdmin && !selectedResource.personal)) {
      setMessage("You can only remove objects you own.");
      return;
    }
    const confirmed = window.confirm(
      selectedResource.personal
        ? "Permanently delete this personal object? It is not archived — this cannot be undone."
        : selectedResource.type === "key_vault_secret"
          ? "Remove this Key Vault object from the app? The Azure secret will not be deleted."
          : selectedResource.type === "app_registration"
            ? "Remove this app registration from the workspace? The Entra app registration will not be deleted."
            : "Remove this object from the app?"
    );
    if (!confirmed) {
      return;
    }
    setBusy(true);
    try {
      await api.archiveResource(selectedResourceId);
      setMessage(selectedResource.personal ? "Personal object permanently deleted" : "Object removed from app");
      await loadAllResources();
      if (session.capabilities.canViewAdmin) {
        await loadArchivedResources();
      }
      if (session.capabilities.canViewAudit) {
        await loadAudit();
      }
      setSelectedResourceId(undefined);
      setSelectedResource(undefined);
      setReveal(null);
      setLaunch(null);
      closeResourceForm();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Remove failed");
    } finally {
      setBusy(false);
    }
  }

  async function handleRestoreArchivedResource(item: ArchivedResourceSummary) {
    if (!session) {
      return;
    }
    const confirmed = window.confirm(`Restore ${item.name} back into the workspace catalog?`);
    if (!confirmed) {
      return;
    }
    setBusy(true);
    try {
      await api.restoreArchivedResource(item.id);
      setMessage(`${item.name} restored to the workspace catalog`);
      await Promise.all([loadAllResources(), loadArchivedResources()]);
      if (session.capabilities.canViewAudit) {
        await loadAudit();
      }
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Restore failed");
    } finally {
      setBusy(false);
    }
  }

  // Sign-out reset: mirrors exactly what App.signOut used to do inline.
  function reset() {
    setReveal(null);
    setLaunch(null);
    setRevealCopyMessage(undefined);
  }

  return {
    passwordOptions,
    connectionOverride,
    reveal,
    setReveal,
    launch,
    setLaunch,
    revealCopyMessage,
    handleReveal,
    handleRevealStoredPassword,
    handleCopyRevealSecret,
    handleLaunch,
    handleSaveConnectionOverride,
    handleClearConnectionOverride,
    handleCreate,
    handleUpdate,
    handleArchive,
    handleRestoreArchivedResource,
    reset
  };
}
