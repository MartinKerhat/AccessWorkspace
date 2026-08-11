import { useEffect, useState, type Dispatch, type SetStateAction } from "react";
import { api } from "../api/client";
import { mentionIdsIn } from "../mentions";
import type {
  ArchivedResourceSummary,
  ConnectionCredentialOverride,
  LaunchPayload,
  LauncherLocalStatus,
  LauncherRuntime,
  MentionTarget,
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
  refreshLauncherStatus: (runtimeArg?: LauncherRuntime | null) => Promise<LauncherLocalStatus | null>;
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
  const [mentionTargets, setMentionTargets] = useState<MentionTarget[]>([]);
  const [reveal, setReveal] = useState<RevealResult | null>(null);
  const [launch, setLaunch] = useState<LaunchPayload | null>(null);
  // Launch-scoped busy state: the desktop-launcher hand-off can legitimately
  // take tens of seconds, and holding the global busy flag for that long
  // disables every button in the app with no feedback. Only the launch
  // buttons react to this.
  const [launching, setLaunching] = useState(false);
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

  // Mentions are resolved per viewer on every read — a verdict cached at write
  // time would drift as sharing changes. Failure is silent: an unresolved
  // mention renders as plain text, which is the safe direction.
  useEffect(() => {
    const notes = selectedResource?.notes ?? "";
    const ids = mentionIdsIn(notes);
    if (!session || ids.length === 0) {
      setMentionTargets([]);
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const response = await api.resolveMentions(ids);
        if (!cancelled) {
          setMentionTargets(response.items);
        }
      } catch {
        if (!cancelled) {
          setMentionTargets([]);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [selectedResource?.id, selectedResource?.notes, session]);

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

  // Reveals the saved password behind a connection's personal credential
  // override. Servers that force their own credential prompt
  // (fPromptForPassword) make manual entry unavoidable, and the password lives
  // in the Passwords module — without this the user has to leave Connections,
  // find the object and reveal it there. Personal passwords are sealed to the
  // session vault key, so ErrVaultLocked has to run the unlock flow first.
  // Reveals an object mentioned in the selected resource's notes. Any id is
  // acceptable here because the API enforces per-viewer access on the target
  // itself — a denied mention never reaches this call, and if it did it would be
  // refused server-side.
  async function handleRevealMentionedObject(resourceID: string) {
    if (!resourceID || !session) {
      return undefined;
    }
    setBusy(true);
    try {
      const response = await api.revealResource(resourceID);
      await refreshAfterSensitiveAction();
      return response.secretValue;
    } catch (error) {
      if (await guardVaultLocked(error, () => handleRevealMentionedObject(resourceID).then(() => undefined))) {
        return undefined;
      }
      setMessage(error instanceof Error ? error.message : "Reveal failed");
      return undefined;
    } finally {
      setBusy(false);
    }
  }

  async function handleRevealOverridePassword() {
    const passwordResourceId = connectionOverride?.passwordResourceId;
    if (!passwordResourceId || !session) {
      return undefined;
    }
    setBusy(true);
    try {
      const response = await api.revealResource(passwordResourceId);
      await refreshAfterSensitiveAction();
      return response.secretValue;
    } catch (error) {
      if (await guardVaultLocked(error, () => handleRevealOverridePassword().then(() => undefined))) {
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
    if (!selectedResourceId || !session || launching) {
      return;
    }
    setLaunching(true);
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
        // Launchers report per-capability support (e.g. Linux RDP needs a
        // FreeRDP client installed) so missing prerequisites surface as a
        // clear message instead of a failed hand-off.
        if (status.capabilities && status.capabilities[selectedResource.type] === false) {
          setMessage(
            selectedResource.type === "rdp"
              ? "This machine's launcher cannot open RDP yet — install the FreeRDP client (e.g. the freerdp package) and try again."
              : "This machine's launcher cannot open SSH yet — install an OpenSSH client and a terminal emulator, then try again."
          );
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
      setLaunching(false);
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
    setLaunching(false);
    setRevealCopyMessage(undefined);
  }

  return {
    passwordOptions,
    connectionOverride,
    reveal,
    setReveal,
    launch,
    setLaunch,
    launching,
    revealCopyMessage,
    handleReveal,
    handleRevealStoredPassword,
    handleRevealOverridePassword,
    handleRevealMentionedObject,
    mentionTargets,
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
