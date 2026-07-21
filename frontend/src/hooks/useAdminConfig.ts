import { useEffect, useState } from "react";
import { api } from "../api/client";
import { emptyKeyVaultSource } from "../keyVault";
import type {
  AdminConfig,
  AdminForm,
  AppRegistrationNotificationPolicy,
  NotificationAdminForm,
  NotificationDeliveryRecord,
  Session
} from "../types";

export function toAdminForm(config: AdminConfig | null): AdminForm {
  return {
    entraTenantId: config?.entraTenantId ?? "",
    entraClientId: config?.entraClientId ?? "",
    entraAuthority: config?.entraAuthority ?? "https://login.microsoftonline.com",
    entraRedirectUri: config?.entraRedirectUri ?? "http://localhost:8080/api/auth/microsoft/callback",
    entraGroupSource: config?.entraGroupSource ?? "graph",
    entraClientSecret: "",
    entraEnabled: config?.entraEnabled ?? false,
    azureReaderUseAmbientIdentity: config?.azureReaderUseAmbientIdentity ?? false,
    rdpSigningEnabled: config?.rdpSigning.enabled ?? false,
    keyVaultSources: config?.keyVaultSources?.map((source) => ({
      ...emptyKeyVaultSource(),
      ...source
    })) ?? []
  };
}

export function defaultAppRegistrationNotificationPolicy(): AppRegistrationNotificationPolicy {
  return {
    enabled: true,
    reminderDays: [30, 14, 7, 3, 1, 0],
    channels: ["in_app"]
  };
}

export function toNotificationAdminForm(config: AdminConfig | null): NotificationAdminForm {
  return {
    appRegistrationNotificationPolicy: config?.appRegistrationNotificationPolicy ?? defaultAppRegistrationNotificationPolicy(),
    notificationEmailEnabled: config?.notificationEmailEnabled ?? false,
    notificationEmailHost: config?.notificationEmailHost ?? "",
    notificationEmailPort: config?.notificationEmailPort ?? 587,
    notificationEmailUsername: config?.notificationEmailUsername ?? "",
    notificationEmailPassword: "",
    notificationEmailFrom: config?.notificationEmailFrom ?? "",
    appRegistrationAutoSyncEnabled: config?.appRegistrationAutoSyncEnabled ?? true,
    appRegistrationSyncIntervalMinutes: config?.appRegistrationSyncIntervalMinutes ?? 60
  };
}

type UseAdminConfigDeps = {
  session: Session | null;
  setBusy: (busy: boolean) => void;
  setMessage: (message: string | undefined) => void;
  // Loading/saving the admin config also decides whether the login page shows
  // the Microsoft button (entraEnabled && entraConfigured).
  onEntraHint: (microsoftLoginHint: boolean) => void;
};

// Admin configuration: the raw config, the Entra/general form, the
// notification-settings form, the email-delivery log, and their save handlers.
export function useAdminConfig({ session, setBusy, setMessage, onEntraHint }: UseAdminConfigDeps) {
  const [adminConfig, setAdminConfig] = useState<AdminConfig | null>(null);
  const [adminForm, setAdminForm] = useState<AdminForm>(toAdminForm(null));
  const [adminModalOpen, setAdminModalOpen] = useState(false);
  const [notificationAdminForm, setNotificationAdminForm] = useState<NotificationAdminForm>(toNotificationAdminForm(null));
  const [notificationDeliveries, setNotificationDeliveries] = useState<NotificationDeliveryRecord[]>([]);

  useEffect(() => {
    setAdminForm(toAdminForm(adminConfig));
    setNotificationAdminForm(toNotificationAdminForm(adminConfig));
  }, [adminConfig]);

  async function loadAdminConfig() {
    const response = await api.adminConfig();
    setAdminConfig(response);
    onEntraHint(response.entraEnabled && response.entraConfigured);
  }

  async function loadNotificationDeliveries() {
    const response = await api.listAdminNotificationDeliveries();
    setNotificationDeliveries(response.items);
  }

  // For handlers outside this hook (Key Vault sources modal) that save the
  // admin config themselves: apply the response the same way the local
  // handlers do.
  function applyAdminConfigResponse(response: AdminConfig) {
    setAdminConfig(response);
    setAdminForm(toAdminForm(response));
  }

  async function handleSaveAdminConfig() {
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
            entraClientSecret: adminForm.entraClientSecret,
            entraEnabled: adminForm.entraEnabled,
            azureReaderUseAmbientIdentity: adminForm.azureReaderUseAmbientIdentity,
            rdpSigningEnabled: adminForm.rdpSigningEnabled,
            keyVaultSources: adminForm.keyVaultSources
          }
      );
      setAdminConfig(response);
      setAdminForm(toAdminForm(response));
      onEntraHint(response.entraEnabled && response.entraConfigured);
      setMessage("Administration settings updated");
      setAdminModalOpen(false);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Saving administration settings failed");
    } finally {
      setBusy(false);
    }
  }

  async function handleSaveNotificationAdminConfig() {
    if (!session) {
      return;
    }
    setBusy(true);
    try {
      const response = await api.updateAdminConfig(
        {
          entraTenantId: adminConfig?.entraTenantId ?? adminForm.entraTenantId,
          entraClientId: adminConfig?.entraClientId ?? adminForm.entraClientId,
          entraAuthority: adminConfig?.entraAuthority ?? adminForm.entraAuthority,
          entraRedirectUri: adminConfig?.entraRedirectUri ?? adminForm.entraRedirectUri,
          entraGroupSource: adminConfig?.entraGroupSource ?? adminForm.entraGroupSource,
          entraEnabled: adminConfig?.entraEnabled ?? adminForm.entraEnabled,
          rdpSigningEnabled: adminConfig?.rdpSigning.enabled ?? adminForm.rdpSigningEnabled,
          keyVaultSources: adminConfig?.keyVaultSources ?? adminForm.keyVaultSources,
          appRegistrationNotificationPolicy: notificationAdminForm.appRegistrationNotificationPolicy,
          notificationEmailEnabled: notificationAdminForm.notificationEmailEnabled,
          notificationEmailHost: notificationAdminForm.notificationEmailHost,
          notificationEmailPort: notificationAdminForm.notificationEmailPort,
          notificationEmailUsername: notificationAdminForm.notificationEmailUsername,
          notificationEmailPassword: notificationAdminForm.notificationEmailPassword,
          notificationEmailFrom: notificationAdminForm.notificationEmailFrom,
          appRegistrationAutoSyncEnabled: notificationAdminForm.appRegistrationAutoSyncEnabled,
          appRegistrationSyncIntervalMinutes: notificationAdminForm.appRegistrationSyncIntervalMinutes
        }
      );
      setAdminConfig(response);
      setNotificationAdminForm(toNotificationAdminForm(response));
      setMessage("Notification settings updated.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to update notification settings");
    } finally {
      setBusy(false);
    }
  }

  async function handleRefreshNotificationDeliveries() {
    if (!session?.capabilities.canViewAdmin) {
      return;
    }
    try {
      setBusy(true);
      await loadNotificationDeliveries();
      setMessage("Notification delivery log refreshed.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to refresh notification delivery log");
    } finally {
      setBusy(false);
    }
  }

  async function handleGenerateRDPSigningTestCertificate() {
    if (!session) {
      return;
    }
    setBusy(true);
    try {
      await api.generateRDPSigningTestCertificate();
      const response = await api.adminConfig();
      setAdminConfig(response);
      setAdminForm(toAdminForm(response));
      setMessage("RDP signing certificate generated.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to generate RDP signing certificate");
    } finally {
      setBusy(false);
    }
  }

  // Sign-out reset: mirrors exactly what App.signOut used to do inline.
  function reset() {
    setAdminConfig(null);
    setAdminForm(toAdminForm(null));
  }

  return {
    adminConfig,
    loadAdminConfig,
    adminForm,
    setAdminForm,
    adminModalOpen,
    setAdminModalOpen,
    notificationAdminForm,
    setNotificationAdminForm,
    notificationDeliveries,
    loadNotificationDeliveries,
    applyAdminConfigResponse,
    handleSaveAdminConfig,
    handleSaveNotificationAdminConfig,
    handleRefreshNotificationDeliveries,
    handleGenerateRDPSigningTestCertificate,
    reset
  };
}
