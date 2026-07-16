import { useEffect, useRef, useState } from "react";
import { api } from "./api/client";
import { ResourceFormModal } from "./modals/ResourceFormModal";
import { AdminConfigModal } from "./modals/AdminConfigModal";
import { ChangePasswordModal } from "./modals/ChangePasswordModal";
import { VaultUnlockModal } from "./modals/VaultUnlockModal";
import { NotificationPolicyModal } from "./modals/NotificationPolicyModal";
import { KeyVaultSourcesModal } from "./modals/KeyVaultSourcesModal";
import { KeyVaultImportModal } from "./modals/KeyVaultImportModal";
import { AppRegistrationImportModal } from "./modals/AppRegistrationImportModal";
import { useLauncher } from "./hooks/useLauncher";
import { useBrowserExtension } from "./hooks/useBrowserExtension";
import { useVault } from "./hooks/useVault";
import { useResourceActions } from "./hooks/useResourceActions";
import { useNotifications } from "./hooks/useNotifications";
import { useAdminUsers } from "./hooks/useAdminUsers";
import { useAdminConfig, defaultAppRegistrationNotificationPolicy } from "./hooks/useAdminConfig";
import { useKeyVaultAdmin, closedKeyVaultModalState } from "./hooks/useKeyVaultAdmin";
import { useAppRegistrationAdmin, closedAppRegistrationModalState } from "./hooks/useAppRegistrationAdmin";
import { ActivityPage } from "./pages/ActivityPage";
import { ArchivedKeyVaultPage } from "./pages/ArchivedKeyVaultPage";
import { AuditPage } from "./pages/AuditPage";
import { CatalogPage } from "./pages/CatalogPage";
import { LocalGroupsAdminPage } from "./pages/LocalGroupsAdminPage";
import { LoginPage } from "./pages/LoginPage";
import { ResourceDetailPage } from "./pages/ResourceDetailPage";
import { UserAccessAdminPage } from "./pages/UserAccessAdminPage";
import { AzureAdminSection } from "./pages/AzureAdminSection";
import { NotificationsAdminSection } from "./pages/NotificationsAdminSection";
import { ConnectionsAdminSection } from "./pages/ConnectionsAdminSection";
import { DiagnosticsAdminSection } from "./pages/DiagnosticsAdminSection";
import { LauncherDownloadsModal } from "./modals/LauncherDownloadsModal";
import { BrowserExtensionManagerModal } from "./modals/BrowserExtensionManagerModal";
import { BrowserExtensionConnectModal } from "./modals/BrowserExtensionConnectModal";
import { RevealSecretModal } from "./modals/RevealSecretModal";
import "./styles/app.css";
import type {
  ArchivedResourceSummary,
  AuditEvent,
  Resource,
  ResourceForm,
  ResourceSummary,
  Session,
  UserNotification
} from "./types";
import {
  categoryLabel,
  filterCategoryItems,
  type WorkspaceCategory
} from "./workspaceCategories";

type View = WorkspaceCategory | "activity" | "audit" | "admin";
type AdminSection = "users" | "groups" | "azure" | "connections" | "notifications" | "model" | "diagnostics";
type Filters = { q: string; target: string };
type FormState =
  | { mode: "closed" }
  | { mode: "create"; draftType: ResourceForm["type"] }
  | { mode: "edit" };
type LoginOptions = {
  localLoginEnabled: boolean;
  microsoftLoginHint: boolean;
};



const defaultFilters: Filters = { q: "", target: "" };
const authTokenStorageKey = "authToken";
const closedFormState: FormState = { mode: "closed" };
const availableRights = [
  "connections.read",
  "connections.edit",
  "connections.create",
  "keyvault.read",
  "keyvault.edit",
  "appregistrations.read",
  "appregistrations.edit",
  "passwords.read",
  "passwords.edit",
  "passwords.create",
  "audit.read",
  "admin.access"
] as const;

function notificationUnreadCount(items: UserNotification[]) {
  return items.filter((item) => !item.readAt).length;
}

function currentView(): View {
  const hash = window.location.hash.replace("#", "");
  if (
    hash === "connections" ||
    hash === "keyvault" ||
    hash === "appregistrations" ||
    hash === "passwords" ||
    hash === "activity" ||
    hash === "audit" ||
    hash === "admin"
  ) {
    return hash;
  }
  return "connections";
}

function pageTitle(view: View): string {
  switch (view) {
    case "connections":
    case "keyvault":
    case "appregistrations":
    case "passwords":
      return categoryLabel(view);
    case "activity":
      return "Recent activity";
    case "audit":
      return "Audit trail";
    case "admin":
      return "Administration";
    default:
      return "Operational access workspace";
  }
}

function loginMessageFromQuery(): string | undefined {
  const params = new URLSearchParams(window.location.search);
  if (params.get("authToken")) {
    return undefined;
  }
  const authError = params.get("authError");

  if (authError) {
    switch (authError) {
      case "microsoft_login_not_available":
        return "Microsoft sign-in is not available yet. Enable and configure it in Administration first.";
      case "microsoft_login_not_configured":
        return "Microsoft sign-in is missing required Entra settings.";
      case "invalid_microsoft_authority":
        return "The configured Microsoft authority URL is invalid.";
      case "invalid_microsoft_state":
        return "Microsoft sign-in could not be completed because the callback state was invalid.";
      case "missing_microsoft_code":
        return "Microsoft sign-in returned without an authorization code.";
      case "microsoft_token_exchange_failed":
        return "Microsoft sign-in reached the callback, but exchanging the authorization code for tokens failed.";
      case "microsoft_user_resolution_failed":
        return "Microsoft sign-in completed token exchange, but the user profile or groups could not be loaded.";
      case "microsoft_session_failed":
        return "Microsoft sign-in completed, but creating the local workspace session failed.";
      case "user_blocked":
        return "This account is blocked from signing in to the workspace.";
      default:
        return "Microsoft sign-in could not be completed.";
    }
  }

  return undefined;
}

function authMessage(error: unknown, fallback: string): string {
  if (!(error instanceof Error)) {
    return fallback;
  }
  if (error.message === "user is blocked") {
    return "This account is blocked from signing in to the workspace.";
  }
  return error.message;
}

function clearLoginQuery() {
  if (!window.location.search) {
    return;
  }
  const next = `${window.location.pathname}${window.location.hash}`;
  window.history.replaceState({}, "", next);
}

// Events are stored forever; the UI pages through them AUDIT_PAGE_SIZE at a time.
const AUDIT_PAGE_SIZE = 100;

export default function App() {
  const [loginOptions, setLoginOptions] = useState<LoginOptions>({
    localLoginEnabled: true,
    microsoftLoginHint: true
  });
  const [session, setSession] = useState<Session | null>(null);
  const [busy, setBusy] = useState(false);
  const [booting, setBooting] = useState(true);
  const [message, setMessage] = useState<string>();
  const [allResources, setAllResources] = useState<ResourceSummary[]>([]);
  const [selectedResourceId, setSelectedResourceId] = useState<string>();
  const [selectedResource, setSelectedResource] = useState<Resource>();
  const [formState, setFormState] = useState<FormState>(closedFormState);
  const {
    launcherRuntime,
    setLauncherRuntime,
    launcherDownloadsOpen,
    setLauncherDownloadsOpen,
    refreshLauncherStatus
  } = useLauncher({ selectedResource });
  const {
    vaultPrompt,
    setVaultPrompt,
    passkeyCapable,
    vaultUnlocked,
    guardVaultLocked,
    toggleVaultLock,
    submitVaultPassphrase,
    submitVaultPasskey,
    reset: resetVault
  } = useVault({ session, setBusy, setMessage });
  const {
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
    reset: resetResourceActions
  } = useResourceActions({
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
    closeResourceForm: () => setFormState(closedFormState)
  });
  const {
    browserExtensionRuntime,
    browserExtensionManagerOpen,
    setBrowserExtensionManagerOpen,
    browserExtensionConnectState,
    setBrowserExtensionConnectState,
    handlePrepareBrowserExtensionSession,
    visibleBrowserPackages,
    currentBrowserPackage
  } = useBrowserExtension({ session, setBusy, setMessage });
  const [activity, setActivity] = useState<AuditEvent[]>([]);
  const {
    notifications,
    loadNotifications,
    handleMarkNotificationRead,
    notificationPolicyModalState,
    setNotificationPolicyModalState,
    handleSaveNotificationPolicyOverride
  } = useNotifications({ session, setBusy, setMessage, setSelectedResource, setAllResources });
  const [audit, setAudit] = useState<AuditEvent[]>([]);
  const [auditHasMore, setAuditHasMore] = useState(false);
  const [auditTotal, setAuditTotal] = useState(0);
  const [auditEventTypes, setAuditEventTypes] = useState<string[]>([]);
  const [auditFilters, setAuditFilters] = useState({ query: "", eventType: "" });
  const {
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
    reset: resetAdminConfig
  } = useAdminConfig({
    session,
    setBusy,
    setMessage,
    onEntraHint: (microsoftLoginHint) => setLoginOptions((current) => ({ ...current, microsoftLoginHint }))
  });
  const [archivedResources, setArchivedResources] = useState<ArchivedResourceSummary[]>([]);
  const {
    localGroups,
    knownUsers,
    selectedAdminUserId,
    setSelectedAdminUserId,
    selectedAdminUser,
    selectedAdminUserResources,
    loadLocalGroups,
    loadKnownUsers,
    handleSaveLocalGroup,
    handleSaveAdminUserAccess,
    handleCreateAdminUser,
    handleResetAdminUserPassword,
    handleDeleteAdminUser,
    reset: resetAdminUsers
  } = useAdminUsers({
    session,
    setBusy,
    setMessage,
    loadAllResources,
    loadAudit,
    refreshCurrentSession,
    onForcedSignOut
  });
  const {
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
    reset: resetKeyVaultAdmin
  } = useKeyVaultAdmin({
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
  });
  const {
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
    reset: resetAppRegistrationAdmin
  } = useAppRegistrationAdmin({
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
  });
  const [filters, setFilters] = useState<Filters>(defaultFilters);
  const [view, setView] = useState<View>(currentView());
  const [adminSection, setAdminSection] = useState<AdminSection>("users");
  const [accountMenuOpen, setAccountMenuOpen] = useState(false);
  const [changePasswordOpen, setChangePasswordOpen] = useState(false);
  const [inviteToken, setInviteToken] = useState<string>(() =>
    new URLSearchParams(window.location.search).get("invite") ?? ""
  );
  const [notificationCenterOpen, setNotificationCenterOpen] = useState(false);
  const notificationMenuRef = useRef<HTMLDivElement | null>(null);
  const accountMenuRef = useRef<HTMLDivElement | null>(null);

  // Close the notification and account popovers on any click outside them (or Escape).
  useEffect(() => {
    if (!notificationCenterOpen && !accountMenuOpen) {
      return;
    }
    function handlePointerDown(event: PointerEvent) {
      const target = event.target as Node | null;
      if (!target) {
        return;
      }
      if (notificationCenterOpen && notificationMenuRef.current && !notificationMenuRef.current.contains(target)) {
        setNotificationCenterOpen(false);
      }
      if (accountMenuOpen && accountMenuRef.current && !accountMenuRef.current.contains(target)) {
        setAccountMenuOpen(false);
      }
    }
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setNotificationCenterOpen(false);
        setAccountMenuOpen(false);
      }
    }
    document.addEventListener("pointerdown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("pointerdown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [notificationCenterOpen, accountMenuOpen]);
  const previousViewRef = useRef<View | null>(null);
  const previousAdminSectionRef = useRef<AdminSection | null>(null);

  const user = session?.user ?? null;

  useEffect(() => {
    const onHashChange = () => setView(currentView());
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  useEffect(() => {
    if (view === "admin") {
      setAdminSection("users");
    }
  }, [view]);

  useEffect(() => {
    if (previousViewRef.current === null) {
      previousViewRef.current = view;
      previousAdminSectionRef.current = adminSection;
      return;
    }

    const viewChanged = previousViewRef.current !== view;
    const adminSectionChanged = view === "admin" && previousAdminSectionRef.current !== adminSection;

    if (viewChanged || adminSectionChanged) {
      setMessage(undefined);
    }

    previousViewRef.current = view;
    previousAdminSectionRef.current = adminSection;
  }, [view, adminSection]);

  useEffect(() => {
    void bootstrapAuth();
  }, []);

  useEffect(() => {
    const loginMessage = loginMessageFromQuery();
    if (loginMessage) {
      setMessage(loginMessage);
      clearLoginQuery();
    }
  }, []);

  useEffect(() => {
    if (!user) {
      return;
    }
    void loadWorkspaceData();
  }, [user]);

  async function bootstrapAuth() {
    setBooting(true);
    try {
      const bootstrap = await api.authBootstrap();
      setLoginOptions({
        localLoginEnabled: bootstrap.localLoginEnabled,
        microsoftLoginHint: bootstrap.microsoftLoginHint
      });
      const params = new URLSearchParams(window.location.search);
      const tokenFromQuery = params.get("authToken") ?? undefined;
      const rememberedToken = tokenFromQuery ?? localStorage.getItem(authTokenStorageKey) ?? undefined;
      if (!rememberedToken) {
        return;
      }
      if (tokenFromQuery) {
        localStorage.setItem(authTokenStorageKey, tokenFromQuery);
        clearLoginQuery();
      }
      const response = await api.authMe(rememberedToken);
      setSession({
        user: response.user,
        authToken: rememberedToken,
        authMode: response.authMode,
        capabilities: response.capabilities
      });
      if (!window.location.hash) {
        window.location.hash = "#connections";
      }
    } catch (error) {
      localStorage.removeItem(authTokenStorageKey);
      const nextMessage = authMessage(error, "Failed to load auth bootstrap");
      if (nextMessage !== "unauthenticated") {
        setMessage(nextMessage);
      } else {
        setMessage(undefined);
      }
    } finally {
      setBooting(false);
    }
  }

  async function signIn(username: string, password: string) {
    setBusy(true);
    try {
      const response = await api.authLogin(username, password);
      localStorage.setItem(authTokenStorageKey, response.token);
      setSession({
        user: response.user,
        authToken: response.token,
        authMode: response.authMode,
        capabilities: response.capabilities
      });
      setMessage(undefined);
      if (!window.location.hash) {
        window.location.hash = "#connections";
      }
    } catch (error) {
      setMessage(authMessage(error, "Sign-in failed"));
    } finally {
      setBusy(false);
    }
  }

  async function acceptInvite(password: string) {
    setBusy(true);
    try {
      const response = await api.acceptInvite(inviteToken, password);
      localStorage.setItem(authTokenStorageKey, response.token);
      setSession({
        user: response.user,
        authToken: response.token,
        authMode: response.authMode,
        capabilities: response.capabilities
      });
      setInviteToken("");
      // Drop the one-time token from the address bar.
      window.history.replaceState(null, "", window.location.pathname + window.location.hash);
      setMessage(undefined);
      if (!window.location.hash) {
        window.location.hash = "#connections";
      }
    } catch (error) {
      setMessage(authMessage(error, "Account setup failed"));
    } finally {
      setBusy(false);
    }
  }

  async function changePassword(currentPassword: string, newPassword: string) {
    if (!session) {
      return false;
    }
    setBusy(true);
    try {
      await api.changePassword(currentPassword, newPassword, session.authToken);
      setMessage("Password changed");
      return true;
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Changing password failed");
      return false;
    } finally {
      setBusy(false);
    }
  }

  function signOut() {
    if (session) {
      void api.authLogout(session.authToken);
    }
    localStorage.removeItem(authTokenStorageKey);
    setBrowserExtensionManagerOpen(false);
    setLauncherDownloadsOpen(false);
    setBrowserExtensionConnectState(null);
    resetVault();
    setSession(null);
    setAllResources([]);
    setSelectedResourceId(undefined);
    setSelectedResource(undefined);
    resetResourceActions();
    setActivity([]);
    setAudit([]);
    setAuditHasMore(false);
    setAuditTotal(0);
    setAuditEventTypes([]);
    setAuditFilters({ query: "", eventType: "" });
    resetAdminConfig();
    setArchivedResources([]);
    resetAdminUsers();
    resetKeyVaultAdmin();
    resetAppRegistrationAdmin();
    setFilters(defaultFilters);
    setView("connections");
    setMessage(undefined);
    setAccountMenuOpen(false);
    setFormState(closedFormState);
    window.location.hash = "";
  }

  async function loadWorkspaceData() {
    if (!session) {
      return;
    }

    try {
      setBusy(true);
      await Promise.all([
        loadAllResources(session.authToken),
        loadActivity(session.authToken),
        loadNotifications(session.authToken),
        session.capabilities.canViewAudit ? loadAudit(session.authToken) : Promise.resolve(),
        session.capabilities.canViewAdmin
          ? Promise.all([
              loadAdminConfig(session.authToken),
              loadArchivedResources(session.authToken),
              loadLocalGroups(session.authToken),
              loadKnownUsers(session.authToken),
              loadNotificationDeliveries(session.authToken)
            ])
          : Promise.resolve()
      ]);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to load workspace");
    } finally {
      setBusy(false);
    }
  }

  async function loadAllResources(authToken: string): Promise<ResourceSummary[]> {
    const response = await api.listResources(new URLSearchParams(), authToken);
    setAllResources(response.items);
    return response.items;
  }

  async function loadResource(id: string, authToken: string) {
    try {
      const item = await api.getResource(id, authToken);
      setSelectedResource(item);
      setReveal(null);
      setLaunch(null);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to load resource");
    }
  }

  async function loadActivity(authToken: string) {
    const response = await api.myActivity(authToken);
    setActivity(response.items);
  }

  async function loadAudit(authToken: string, filters = auditFilters) {
    const response = await api.listAudit(authToken, {
      limit: AUDIT_PAGE_SIZE,
      offset: 0,
      query: filters.query,
      eventType: filters.eventType
    });
    setAudit(response.items);
    setAuditTotal(response.total);
    setAuditEventTypes(response.eventTypes);
    setAuditHasMore(response.items.length < response.total);
  }

  async function loadOlderAudit() {
    if (!session) {
      return;
    }
    const response = await api.listAudit(session.authToken, {
      limit: AUDIT_PAGE_SIZE,
      offset: audit.length,
      query: auditFilters.query,
      eventType: auditFilters.eventType
    });
    setAudit((current) => {
      const seen = new Set(current.map((item) => item.id));
      const merged = [...current, ...response.items.filter((item) => !seen.has(item.id))];
      setAuditHasMore(merged.length < response.total);
      return merged;
    });
    setAuditTotal(response.total);
  }

  function handleAuditFiltersChange(filters: { query: string; eventType: string }) {
    setAuditFilters(filters);
    if (session?.capabilities.canViewAudit) {
      void loadAudit(session.authToken, filters);
    }
  }

  async function loadArchivedResources(authToken: string) {
    const response = await api.listArchivedResources(authToken);
    setArchivedResources(response.items);
  }

  async function refreshCurrentSession(authToken: string) {
    const response = await api.authMe(authToken);
    setSession({
      user: response.user,
      authToken,
      authMode: response.authMode,
      capabilities: response.capabilities
    });
    return response;
  }

  // Invoked by useAdminUsers when refreshing the current user's own session
  // fails after an access change: clear everything App still owns (the hook
  // clears its own state before calling this).
  function onForcedSignOut(failureMessage: string) {
    localStorage.removeItem(authTokenStorageKey);
    setSession(null);
    setAllResources([]);
    setSelectedResourceId(undefined);
    setSelectedResource(undefined);
    resetResourceActions();
    setActivity([]);
    setAudit([]);
    resetAdminConfig();
    setArchivedResources([]);
    resetKeyVaultAdmin();
    resetAppRegistrationAdmin();
    setFilters(defaultFilters);
    setView("connections");
    setAccountMenuOpen(false);
    setFormState(closedFormState);
    window.location.hash = "";
    setMessage(failureMessage);
  }

  const visibleCategories = (["connections", "keyvault", "appregistrations", "passwords"] as WorkspaceCategory[]).filter(
    (category) => session?.capabilities.categories[category]?.view
  );
  const categoryView: WorkspaceCategory | null = view === "activity" || view === "audit" || view === "admin" ? null : view;
  const categoryItems = categoryView ? filterCategoryItems(allResources, categoryView) : [];
  const currentItems = categoryItems.filter((item) => {
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
  const archivedKeyVaultItems = archivedResources.filter((item) => item.type === "key_vault_secret");
  const currentArchivedKeyVaultItems = archivedKeyVaultItems.filter((item) => {
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
  useEffect(() => {
    if (view === "admin" && session && !session.capabilities.canViewAdmin) {
      const fallback = visibleCategories[0] ?? "activity";
      window.location.hash = `#${fallback}`;
      return;
    }
    if (view === "audit" && session && !session.capabilities.canViewAudit) {
      const fallback = visibleCategories[0] ?? "activity";
      window.location.hash = `#${fallback}`;
    }
  }, [session, view, visibleCategories]);

  useEffect(() => {
    if (!categoryView) {
      return;
    }
    setFilters(defaultFilters);
  }, [categoryView]);

  useEffect(() => {
    if (categoryView !== "keyvault" || keyVaultViewMode !== "archived") {
      return;
    }
    if (!selectedArchivedKeyVaultId && currentArchivedKeyVaultItems.length > 0) {
      setSelectedArchivedKeyVaultId(currentArchivedKeyVaultItems[0].id);
      return;
    }
    if (selectedArchivedKeyVaultId && !currentArchivedKeyVaultItems.some((item) => item.id === selectedArchivedKeyVaultId)) {
      setSelectedArchivedKeyVaultId(currentArchivedKeyVaultItems[0]?.id);
    }
  }, [categoryView, keyVaultViewMode, currentArchivedKeyVaultItems, selectedArchivedKeyVaultId]);

  useEffect(() => {
    if (view === "activity" || view === "audit" || view === "admin") {
      return;
    }
    if (!visibleCategories.includes(view)) {
      const fallback = visibleCategories[0] ?? "activity";
      window.location.hash = `#${fallback}`;
      return;
    }
    if (!selectedResourceId && currentItems.length > 0) {
      setSelectedResourceId(currentItems[0].id);
      return;
    }
    if (selectedResourceId && !currentItems.some((item) => item.id === selectedResourceId)) {
      setSelectedResourceId(currentItems[0]?.id);
    }
  }, [view, visibleCategories, currentItems, selectedResourceId]);

  useEffect(() => {
    setAccountMenuOpen(false);
  }, [view]);

  useEffect(() => {
    setFormState(closedFormState);
  }, [view]);

  function defaultTypeForCategory(category: WorkspaceCategory): ResourceForm["type"] {
    switch (category) {
      case "connections":
        return "ssh";
      case "passwords":
        return "shared_secret";
      case "keyvault":
        return "key_vault_secret";
      case "appregistrations":
        return "app_registration";
    }
  }

  function createLabelForCategory(category: WorkspaceCategory): string {
    switch (category) {
      case "connections":
        return "Create connection";
      case "passwords":
        return "Create password";
      case "keyvault":
        return "Import Key Vault object";
      case "appregistrations":
        return "Import app registration";
    }
  }

  function canCreateInCategory(category: WorkspaceCategory): boolean {
    const capabilities = session?.capabilities.categories[category];
    if (!capabilities) {
      return false;
    }
    // Passwords and connections are self-service: the create capability alone
    // is enough (connections.create makes the creator the owner, shared by default).
    if (category === "passwords" || category === "connections") {
      return Boolean(capabilities.create);
    }
    return Boolean(session?.user.isAdmin && (capabilities.create || capabilities.import));
  }

  function handleMicrosoftSignIn() {
    window.location.assign(api.microsoftStartUrl());
  }

  const adminSections: Array<{ id: AdminSection; label: string }> = [
    { id: "users", label: "Users" },
    { id: "groups", label: "Groups" },
    { id: "azure", label: "Azure" },
    { id: "connections", label: "Connections" },
    { id: "notifications", label: "Notifications" },
    { id: "model", label: "Model" },
    { id: "diagnostics", label: "Diagnostics" }
  ];

  if (booting) {
    return (
      <div className="loading-screen">
        <div className="loading-card">
          <p className="eyebrow">Access Workspace</p>
          <h1>Preparing workspace</h1>
          <p className="section-copy">Loading saved session state and backend access policy.</p>
        </div>
      </div>
    );
  }

  if (!session) {
    return (
      <LoginPage
        loading={busy}
        message={message}
        microsoftEnabled={loginOptions.microsoftLoginHint}
        inviteToken={inviteToken || undefined}
        onSignIn={signIn}
        onMicrosoftSignIn={handleMicrosoftSignIn}
        onAcceptInvite={acceptInvite}
      />
    );
  }

  const currentUser = session.user;

  return (
    <div className="workspace-shell">
      <aside className="workspace-sidebar">
        <div className="brand-block">
          <p className="eyebrow">Internal access</p>
          <h1>Access Workspace</h1>
          <p className="section-copy">
            Discover shared operational access, use approved actions, and build toward one governed launcher and secret workspace.
          </p>
        </div>

        <nav className="nav-list">
          {visibleCategories.map((category) => (
            <a key={category} className={view === category ? "active" : ""} href={`#${category}`}>
              {categoryLabel(category)}
            </a>
          ))}
          {session.capabilities.canViewActivity ? (
            <a className={view === "activity" ? "active" : ""} href="#activity">
              Activity
            </a>
          ) : null}
          {session.capabilities.canViewAudit ? (
            <a className={view === "audit" ? "active" : ""} href="#audit">
              Audit
            </a>
          ) : null}
          {session.capabilities.canViewAdmin ? (
            <a className={view === "admin" ? "active" : ""} href="#admin">
              Admin
            </a>
          ) : null}
        </nav>

      </aside>

      <main className="workspace-main">
        <header className="workspace-topbar">
          <div>
            <p className="eyebrow">Workspace</p>
            <h2>{pageTitle(view)}</h2>
          </div>
          <div className="topbar-actions">
            {categoryView === "keyvault" && session.user.isAdmin ? (
              <div className="segmented-control topbar-segmented-control" role="tablist" aria-label="Key Vault view mode">
                <button
                  type="button"
                  className={`segmented-button ${keyVaultViewMode === "active" ? "active" : ""}`}
                  onClick={() => setKeyVaultViewMode("active")}
                >
                  Active
                </button>
                <button
                  type="button"
                  className={`segmented-button ${keyVaultViewMode === "archived" ? "active" : ""}`}
                  onClick={() => setKeyVaultViewMode("archived")}
                >
                  Archived
                </button>
              </div>
            ) : null}
            <div className="account-menu" ref={notificationMenuRef}>
              <button
                className={`session-chip button ghost notification-chip ${notificationUnreadCount(notifications) > 0 ? "has-unread" : ""}`}
                onClick={() => setNotificationCenterOpen((open) => !open)}
              >
                <span className="notification-chip-copy">
                  <span className="notification-chip-title">Notifications</span>
                  <small className="notification-chip-count">{notificationUnreadCount(notifications)} unread</small>
                </span>
              </button>
              {notificationCenterOpen ? (
                <div className="account-popover notification-popover">
                  <p className="eyebrow">Notification center</p>
                  {notifications.length === 0 ? (
                    <p className="section-copy">No app registration reminders yet.</p>
                  ) : (
                    <div className="notification-list">
                      {notifications.map((item) => (
                        <button
                          key={item.id}
                          type="button"
                          className={`notification-item ${item.readAt ? "read" : "unread"}`}
                          onClick={() => {
                            window.location.hash = "#appregistrations";
                            setSelectedResourceId(item.resourceId);
                            setNotificationCenterOpen(false);
                            void handleMarkNotificationRead(item.id);
                          }}
                        >
                          <div>
                            <strong>{item.title}</strong>
                            <p>{item.body}</p>
                            <p>{new Date(item.createdAt).toLocaleString()}</p>
                            {item.channels.includes("email") ? (
                              <p>
                                email {item.emailStatus || "pending"}
                                {item.emailError ? `: ${item.emailError}` : ""}
                              </p>
                            ) : null}
                          </div>
                          {!item.readAt ? <span className="tag">new</span> : null}
                        </button>
                      ))}
                    </div>
                  )}
                </div>
              ) : null}
            </div>
            <div className="account-menu" ref={accountMenuRef}>
              <button className="session-chip button ghost" onClick={() => setAccountMenuOpen((open) => !open)}>
                <span>{currentUser.name}</span>
                <small>{currentUser.isAdmin ? "Admin session" : "Member session"}</small>
              </button>
              {accountMenuOpen ? (
                <div className="account-popover">
                  <p className="eyebrow">Signed in</p>
                  <strong>{currentUser.name}</strong>
                  <span>{currentUser.email}</span>
                  <span>{currentUser.isAdmin ? "Administrator" : "Standard user"}</span>
                  {session.capabilities.categories.passwords.view ? (
                    <button
                      className="button ghost"
                      onClick={() => {
                        setAccountMenuOpen(false);
                        setBrowserExtensionManagerOpen(true);
                      }}
                    >
                      Browser extensions
                    </button>
                  ) : null}
                  <button
                    className="button ghost"
                    onClick={() => {
                      setAccountMenuOpen(false);
                      void toggleVaultLock();
                    }}
                  >
                    {vaultUnlocked ? "Lock personal passwords" : "Unlock personal passwords"}
                  </button>
                  <button
                    className="button ghost"
                    onClick={() => {
                      setAccountMenuOpen(false);
                      setChangePasswordOpen(true);
                    }}
                  >
                    Change password
                  </button>
                  <button className="button ghost" onClick={signOut}>
                    Sign out
                  </button>
                </div>
              ) : null}
            </div>
          </div>
        </header>

        {message ? <div className="banner">{message}</div> : null}

        {categoryView ? (
          <>
            {categoryView === "keyvault" && session.user.isAdmin && keyVaultViewMode === "archived" ? (
              <ArchivedKeyVaultPage
                filters={filters}
                items={currentArchivedKeyVaultItems}
                selectedId={selectedArchivedKeyVaultId}
                loading={busy}
                onFilterChange={setFilters}
                onSelect={setSelectedArchivedKeyVaultId}
                onRestore={(item) => void handleRestoreArchivedResource(item)}
              />
            ) : (
              <div className="workspace-grid">
                <CatalogPage
                  category={categoryView}
                  filters={filters}
                  items={currentItems}
                  selectedId={selectedResourceId}
                  canCreate={canCreateInCategory(categoryView)}
                  createLabel={createLabelForCategory(categoryView)}
                  secondaryActionLabel={
                    categoryView === "keyvault" && session.user.isAdmin
                      ? keyVaultSyncing ? "Syncing..." : "Sync now"
                      : categoryView === "appregistrations" && session.user.isAdmin
                        ? appRegistrationSyncing ? "Syncing..." : "Sync now"
                        : undefined
                  }
                  onFilterChange={setFilters}
                  onSelect={setSelectedResourceId}
                  onSecondaryAction={
                    categoryView === "keyvault" && session.user.isAdmin
                      ? () => {
                          void handleSyncKeyVault(false);
                        }
                      : categoryView === "appregistrations" && session.user.isAdmin
                        ? () => {
                            void handleSyncAppRegistrations(false);
                          }
                      : undefined
                  }
                  onCreate={() => {
                    if (categoryView === "keyvault") {
                      void openKeyVaultImport();
                      return;
                    }
                    if (categoryView === "appregistrations") {
                      void openAppRegistrationImport();
                      return;
                    }
                    setFormState({ mode: "create", draftType: defaultTypeForCategory(categoryView) });
                  }}
                />
                <ResourceDetailPage
                  resource={selectedResource}
                  launch={launch}
                  loading={busy}
                  canEdit={Boolean(
                    selectedResource &&
                      // Owners always manage their own objects (even without the
                      // category edit right). Shared objects: any visible holder
                      // of the edit right may open the form (non-owners get
                      // metadata-only fields). Personal objects stay owner-only.
                      (selectedResource.ownerUserId === session.user.id ||
                        (!selectedResource.personal &&
                          session.capabilities.categories[selectedResource.category]?.edit))
                  )}
                  canRemove={Boolean(
                    selectedResource &&
                    (selectedResource.ownerUserId === session.user.id ||
                      (!selectedResource.personal && session.user.isAdmin))
                  )}
                  canOverrideRevealPolicy={Boolean(
                    selectedResource &&
                      selectedResource.category === "passwords" &&
                      (selectedResource.ownerUserId === session.user.id ||
                        (!selectedResource.personal && session.user.isAdmin))
                  )}
                  launcherRuntime={launcherRuntime}
                  browserExtensionRuntime={browserExtensionRuntime}
                  passwordOptions={passwordOptions}
                  connectionOverride={connectionOverride}
                  onEdit={() => setFormState({ mode: "edit" })}
                  onEditNotifications={() => {
                    if (!selectedResource || selectedResource.type !== "app_registration") {
                      return;
                    }
                    setNotificationPolicyModalState({
                      mode: "resource",
                      resource: selectedResource,
                      useResourceOverride: Boolean(selectedResource.appNotificationPolicyOverride),
                      draft: selectedResource.appNotificationPolicyOverride ?? adminConfig?.appRegistrationNotificationPolicy ?? defaultAppRegistrationNotificationPolicy(),
                      credentialDrafts: (selectedResource.appCredentials ?? [])
                        .filter((item) => item.notificationPolicyOverride)
                        .map((item) => ({
                          keyId: item.keyId,
                          policy: item.notificationPolicyOverride
                        }))
                    });
                  }}
                  onRemove={handleArchive}
                  onPrepareBrowserExtension={handlePrepareBrowserExtensionSession}
                  onOpenBrowserExtensions={() => setBrowserExtensionManagerOpen(true)}
                  onOpenLauncherDownloads={() => setLauncherDownloadsOpen(true)}
                  onReveal={handleReveal}
                  onLaunch={handleLaunch}
                  onSaveConnectionOverride={handleSaveConnectionOverride}
                  onClearConnectionOverride={handleClearConnectionOverride}
                />
              </div>
            )}
          </>
        ) : null}

        {view === "activity" && session.capabilities.canViewActivity ? (
          <ActivityPage
            title="My recent activity"
            description="Views, reveals, and launches performed during this development session."
            items={activity}
          />
        ) : null}

        {view === "audit" && session.capabilities.canViewAudit ? (
          <AuditPage
            items={audit}
            total={auditTotal}
            eventTypes={auditEventTypes}
            hasMore={auditHasMore}
            onLoadOlder={() => void loadOlderAudit()}
            onFiltersChange={handleAuditFiltersChange}
          />
        ) : null}

        {view === "admin" && session.capabilities.canViewAdmin ? (
          <div className="admin-layout">
            <div className="admin-nav-strip" role="tablist" aria-label="Administration sections">
              {adminSections.map((section) => (
                <button
                  key={section.id}
                  type="button"
                  className={`admin-nav-button ${adminSection === section.id ? "active" : ""}`}
                  onClick={() => setAdminSection(section.id)}
                >
                  {section.label}
                </button>
              ))}
            </div>

            {adminSection === "users" ? (
              <UserAccessAdminPage
                items={knownUsers}
                selectedId={selectedAdminUserId}
                selectedUser={selectedAdminUser}
                visibleResources={selectedAdminUserResources}
                availableGroups={localGroups}
                availableRights={availableRights}
                currentUserId={currentUser.id}
                loading={busy}
                onSelect={setSelectedAdminUserId}
                onCreate={(input) => handleCreateAdminUser(input)}
                onSave={(input) => void handleSaveAdminUserAccess(input)}
                onDelete={(target) => void handleDeleteAdminUser(target)}
                onResetPassword={(target) => handleResetAdminUserPassword(target)}
              />
            ) : null}

            {adminSection === "groups" ? (
              <LocalGroupsAdminPage
                items={localGroups}
                availableRights={availableRights}
                availableUsers={knownUsers}
                loading={busy}
                onSave={(mode, originalName, input) => handleSaveLocalGroup(mode, originalName, input)}
              />
            ) : null}

            {adminSection === "azure" ? (
              <AzureAdminSection
                adminConfig={adminConfig}
                sessionAuthMode={session.authMode}
                isAdmin={session.user.isAdmin}
                busy={busy}
                keyVaultSyncing={keyVaultSyncing}
                notificationAdminForm={notificationAdminForm}
                setNotificationAdminForm={setNotificationAdminForm}
                onEditEntra={() => setAdminModalOpen(true)}
                onSyncKeyVault={() => void handleSyncKeyVault(false)}
                onEditKeyVaultSources={() => setKeyVaultModalState({ mode: "sources" })}
                onSaveAutoSync={() => void handleSaveNotificationAdminConfig()}
              />
            ) : null}

            {adminSection === "connections" ? (
              <ConnectionsAdminSection
                adminConfig={adminConfig}
                rdpSigningEnabled={adminForm.rdpSigningEnabled}
                onRdpSigningEnabledChange={(checked) =>
                  setAdminForm((current) => ({ ...current, rdpSigningEnabled: checked }))
                }
                busy={busy}
                onSave={() => void handleSaveAdminConfig()}
                onGenerateTestCertificate={() => void handleGenerateRDPSigningTestCertificate()}
              />
            ) : null}

            {adminSection === "notifications" ? (
              <NotificationsAdminSection
                form={notificationAdminForm}
                setForm={setNotificationAdminForm}
                emailConfigured={Boolean(adminConfig?.notificationEmailConfigured)}
                emailPasswordSet={Boolean(adminConfig?.notificationEmailPasswordSet)}
                busy={busy}
                deliveries={notificationDeliveries}
                onSaveSettings={() => void handleSaveNotificationAdminConfig()}
                onRefreshLog={() => void handleRefreshNotificationDeliveries()}
              />
            ) : null}

            {adminSection === "model" ? (
              <section className="panel">
                <div className="panel-header">
                  <div>
                    <p className="eyebrow">Workspace model</p>
                    <h2>Object handling</h2>
                  </div>
                </div>
                <p className="section-copy">
                  Connections and Passwords are created from their category pages. Key Vault and App registrations are linked/imported source records with local ownership, visibility, and notes overlays.
                </p>
              </section>
            ) : null}

            {adminSection === "diagnostics" ? <DiagnosticsAdminSection user={currentUser} /> : null}
          </div>
        ) : null}

        {formState.mode !== "closed" ? (
          <ResourceFormModal
            mode={formState.mode}
            headingName={
              formState.mode === "create"
                ? "Category-managed object flow"
                : selectedResource?.name ?? "Edit selected object"
            }
            resource={formState.mode === "edit" ? selectedResource : undefined}
            initialType={formState.mode === "create" ? formState.draftType : undefined}
            availableGroups={localGroups.map((group) => group.name)}
            availableOwners={knownUsers}
            defaultPersonalPassword={!session.user.isAdmin}
            canAssignOwner={session.user.isAdmin}
            sharedMetadataOnly={
              formState.mode === "edit" &&
              !session.user.isAdmin &&
              Boolean(
                selectedResource &&
                  !selectedResource.personal &&
                  selectedResource.ownerUserId !== session.user.id
              )
            }
            loading={busy}
            onSubmit={formState.mode === "create" ? handleCreate : handleUpdate}
            onRevealStoredPassword={
              formState.mode === "edit" &&
              selectedResource &&
              selectedResource.category === "passwords" &&
              ((!selectedResource.personal && session.user.isAdmin) ||
                selectedResource.ownerUserId === session.user.id)
                ? handleRevealStoredPassword
                : undefined
            }
            onArchive={
              formState.mode === "edit" &&
              selectedResource &&
              ((!selectedResource.personal && session.user.isAdmin) ||
                selectedResource.ownerUserId === session.user.id)
                ? handleArchive
                : undefined
            }
            onClose={() => setFormState(closedFormState)}
          />
        ) : null}

        {changePasswordOpen ? (
          <ChangePasswordModal busy={busy} onSave={changePassword} onClose={() => setChangePasswordOpen(false)} />
        ) : null}

        {vaultPrompt ? (
          <VaultUnlockModal
            hasVault={vaultPrompt.status.hasVault}
            passkeyCapable={passkeyCapable}
            hasPasskey={vaultPrompt.status.passkeys.length > 0}
            busy={busy}
            onPasskey={submitVaultPasskey}
            onPassphrase={submitVaultPassphrase}
            onCancel={() => setVaultPrompt(null)}
          />
        ) : null}

        {adminModalOpen ? (
          <AdminConfigModal
            form={adminForm}
            setForm={setAdminForm}
            clientSecretSet={Boolean(adminConfig?.entraClientSecretSet)}
            busy={busy}
            onSave={() => void handleSaveAdminConfig()}
            onClose={() => setAdminModalOpen(false)}
          />
        ) : null}

        {keyVaultModalState.mode === "sources" ? (
          <KeyVaultSourcesModal
            sources={adminForm.keyVaultSources}
            setSources={(updater) =>
              setAdminForm((current) => ({ ...current, keyVaultSources: updater(current.keyVaultSources) }))
            }
            knownUsers={knownUsers}
            localGroups={localGroups}
            busy={busy}
            onSave={() => void handleSaveKeyVaultSources()}
            onClose={() => setKeyVaultModalState(closedKeyVaultModalState)}
          />
        ) : null}

        {keyVaultModalState.mode === "import" ? (
          <KeyVaultImportModal
            discoveries={keyVaultDiscoveries}
            form={keyVaultImportForm}
            setForm={setKeyVaultImportForm}
            knownUsers={knownUsers}
            localGroups={localGroups}
            busy={busy}
            onRefresh={() => {
              if (session) {
                void loadKeyVaultDiscoveries(session.authToken);
              }
            }}
            onImport={() => void handleImportKeyVaultSecret()}
            onClose={() => setKeyVaultModalState(closedKeyVaultModalState)}
          />
        ) : null}

        {appRegistrationModalState.mode === "import" ? (
          <AppRegistrationImportModal
            discoveries={appRegistrationDiscoveries}
            form={appRegistrationImportForm}
            setForm={setAppRegistrationImportForm}
            knownUsers={knownUsers}
            localGroups={localGroups}
            importedAppIds={importedAppRegistrationIds}
            tenantLabel={adminConfig?.entraTenantId || "Configured tenant"}
            authorityLabel={adminConfig?.entraAuthority || "Microsoft Graph"}
            busy={busy}
            onRefresh={() => {
              if (session) {
                void loadAppRegistrationDiscoveries(session.authToken);
              }
            }}
            onImport={() => void handleImportAppRegistrations()}
            onClose={() => setAppRegistrationModalState(closedAppRegistrationModalState)}
          />
        ) : null}

        {notificationPolicyModalState.mode === "resource" ? (
          <NotificationPolicyModal
            state={notificationPolicyModalState}
            setState={setNotificationPolicyModalState}
            busy={busy}
            onSave={() => void handleSaveNotificationPolicyOverride()}
            onClose={() => setNotificationPolicyModalState({ mode: "closed" })}
          />
        ) : null}

        {launcherDownloadsOpen && launcherRuntime ? (
          <LauncherDownloadsModal runtime={launcherRuntime} onClose={() => setLauncherDownloadsOpen(false)} />
        ) : null}

        {browserExtensionManagerOpen && browserExtensionRuntime ? (
          <BrowserExtensionManagerModal
            visiblePackages={visibleBrowserPackages}
            currentPackage={currentBrowserPackage}
            busy={busy}
            onClose={() => setBrowserExtensionManagerOpen(false)}
            onConnect={() => void handlePrepareBrowserExtensionSession()}
          />
        ) : null}

        {browserExtensionConnectState ? (
          <BrowserExtensionConnectModal
            connectState={browserExtensionConnectState}
            hasRuntime={Boolean(browserExtensionRuntime)}
            onClose={() => setBrowserExtensionConnectState(null)}
            onOpenManager={() => setBrowserExtensionManagerOpen(true)}
            onRetry={() => void handlePrepareBrowserExtensionSession()}
          />
        ) : null}

        {reveal?.secretValue ? (
          <RevealSecretModal
            title={selectedResource?.name ?? "Revealed secret"}
            secretValue={reveal.secretValue}
            copyMessage={revealCopyMessage}
            onClose={() => setReveal(null)}
            onCopy={() => void handleCopyRevealSecret()}
          />
        ) : null}
      </main>
    </div>
  );
}
