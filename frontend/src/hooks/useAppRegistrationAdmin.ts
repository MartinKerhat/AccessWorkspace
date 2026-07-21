import { useState, type Dispatch, type SetStateAction } from "react";
import { api } from "../api/client";
import { getSelectedAppRegistrationItems } from "../appRegistration";
import type {
  AdminConfig,
  AdminForm,
  AppRegistrationDiscoverResult,
  AppRegistrationImportForm,
  AppRegistrationSyncResult,
  LaunchPayload,
  Resource,
  ResourceSummary,
  RevealResult,
  Session
} from "../types";

export type AppRegistrationModalState =
  | { mode: "closed" }
  | { mode: "import" };

export const closedAppRegistrationModalState: AppRegistrationModalState = { mode: "closed" };

function emptyAppRegistrationImportForm(): AppRegistrationImportForm {
  return {
    owner: "",
    ownerTeam: "",
    environment: "",
    description: "",
    notes: "",
    allowedGroups: [],
    selectedApplicationIds: []
  };
}

function summarizeAppRegistrationSync(result: AppRegistrationSyncResult): string {
  if (result.attemptedResources === 0) {
    return "No imported app registrations needed syncing.";
  }
  const parts = [`updated ${result.updatedResources} app registrations`];
  if (result.removedResources > 0) {
    parts.push(`removed ${result.removedResources} missing apps`);
  }
  if (result.expiringCredentials > 0) {
    parts.push(`${result.expiringCredentials} credentials expiring within 30 days`);
  }
  if (result.expiredCredentials > 0) {
    parts.push(`${result.expiredCredentials} credentials already expired`);
  }
  if (result.missingResources > 0) {
    parts.push(`${result.missingResources} need attention`);
  }
  return parts.join(", ");
}

type UseAppRegistrationAdminDeps = {
  session: Session | null;
  setBusy: (busy: boolean) => void;
  setMessage: (message: string | undefined) => void;
  adminConfig: AdminConfig | null;
  adminForm: AdminForm;
  allResources: ResourceSummary[];
  loadAdminConfig: () => Promise<void>;
  loadArchivedResources: () => Promise<void>;
  loadAllResources: () => Promise<{ id: string }[]>;
  loadActivity: () => Promise<void>;
  loadAudit: () => Promise<void>;
  loadNotifications: () => Promise<void>;
  loadResource: (id: string) => Promise<void>;
  selectedResourceId: string | undefined;
  setSelectedResourceId: Dispatch<SetStateAction<string | undefined>>;
  setSelectedResource: Dispatch<SetStateAction<Resource | undefined>>;
  setReveal: Dispatch<SetStateAction<RevealResult | null>>;
  setLaunch: Dispatch<SetStateAction<LaunchPayload | null>>;
};

// App-registration administration: discovery, the import modal, manual /
// automatic sync, and the import flow.
export function useAppRegistrationAdmin({
  session,
  setBusy,
  setMessage,
  adminConfig,
  adminForm,
  allResources,
  loadAdminConfig,
  loadArchivedResources,
  loadAllResources,
  loadActivity,
  loadAudit,
  loadNotifications,
  loadResource,
  selectedResourceId,
  setSelectedResourceId,
  setSelectedResource,
  setReveal,
  setLaunch
}: UseAppRegistrationAdminDeps) {
  const [appRegistrationDiscoveries, setAppRegistrationDiscoveries] = useState<AppRegistrationDiscoverResult>({ items: [] });
  const [appRegistrationImportForm, setAppRegistrationImportForm] = useState<AppRegistrationImportForm>(emptyAppRegistrationImportForm());
  const [appRegistrationSyncing, setAppRegistrationSyncing] = useState(false);
  const [appRegistrationModalState, setAppRegistrationModalState] = useState<AppRegistrationModalState>(closedAppRegistrationModalState);

  const importedAppRegistrationIds = new Set(
    allResources
      .filter((item) => item.type === "app_registration")
      .flatMap((item) => [item.applicationId])
      .filter(Boolean)
  );

  async function loadAppRegistrationDiscoveries() {
    const response = await api.discoverAppRegistrations();
    setAppRegistrationDiscoveries({
      items: (response.items ?? []).map((item) => ({
        ...item,
        owners: item.owners ?? [],
        credentials: item.credentials ?? []
      }))
    });
  }

  async function handleSyncAppRegistrations(automatic: boolean) {
    if (!session || appRegistrationSyncing) {
      return;
    }
    if (!session.user.isAdmin) {
      if (!automatic) {
        setMessage("Only admins can sync app registration metadata");
      }
      return;
    }
    setAppRegistrationSyncing(true);
    try {
      const result = await api.syncAppRegistrations(automatic);
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
      if (session.capabilities.canViewAdmin && result.removedResources > 0) {
        await loadArchivedResources();
      }
      await loadNotifications();
      if (session.capabilities.canViewAdmin) {
        await loadAdminConfig();
      }
      if (!automatic) {
        setMessage(summarizeAppRegistrationSync(result));
      }
    } catch (error) {
      if (!automatic) {
        setMessage(error instanceof Error ? error.message : "App registration sync failed");
      }
    } finally {
      setAppRegistrationSyncing(false);
    }
  }

  async function openAppRegistrationImport() {
    if (!session) {
      return;
    }
    setBusy(true);
    try {
      await loadAppRegistrationDiscoveries();
      setAppRegistrationImportForm(emptyAppRegistrationImportForm());
      setAppRegistrationModalState({ mode: "import" });
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "App registration discovery failed");
    } finally {
      setBusy(false);
    }
  }

  async function handleImportAppRegistrations() {
    if (!session) {
      return;
    }
    setBusy(true);
    try {
      const response = await api.importAppRegistrations(
        {
          ...appRegistrationImportForm,
          selectedApplicationIds: getSelectedAppRegistrationItems(
            appRegistrationImportForm.selectedApplicationIds,
            appRegistrationDiscoveries.items
          )
            .filter((item) => !importedAppRegistrationIds.has(item.appId))
            .map((item) => item.id),
          tenantId: adminConfig?.entraTenantId ?? adminForm.entraTenantId
        }
      );
      const createdItems = response.items ?? [];
      if (createdItems.length === 0) {
        setMessage("Selected app registrations were already imported");
      } else {
        setMessage(createdItems.length === 1 ? "App registration imported" : `${createdItems.length} app registrations imported`);
      }
      await loadAllResources();
      await loadActivity();
      if (session.capabilities.canViewAudit) {
        await loadAudit();
      }
      if (createdItems[0]) {
        setSelectedResourceId(createdItems[0].id);
        await loadResource(createdItems[0].id);
      }
      setAppRegistrationModalState(closedAppRegistrationModalState);
      setAppRegistrationImportForm(emptyAppRegistrationImportForm());
      window.location.hash = "#appregistrations";
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Importing app registrations failed");
    } finally {
      setBusy(false);
    }
  }

  // Sign-out reset: mirrors exactly what App.signOut used to do inline.
  function reset() {
    setAppRegistrationDiscoveries({ items: [] });
    setAppRegistrationImportForm(emptyAppRegistrationImportForm());
    setAppRegistrationSyncing(false);
    setAppRegistrationModalState(closedAppRegistrationModalState);
  }

  return {
    appRegistrationDiscoveries,
    loadAppRegistrationDiscoveries,
    appRegistrationImportForm,
    setAppRegistrationImportForm,
    appRegistrationSyncing,
    appRegistrationModalState,
    setAppRegistrationModalState,
    importedAppRegistrationIds,
    handleSyncAppRegistrations,
    openAppRegistrationImport,
    handleImportAppRegistrations,
    reset
  };
}
