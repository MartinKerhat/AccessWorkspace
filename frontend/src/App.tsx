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
import { currentView, type View } from "./navigation";
import { useAuth, authTokenStorageKey } from "./hooks/useAuth";
import { filterCatalogItems, filterArchivedKeyVaultItems, defaultFilters, type Filters } from "./catalogFilter";
import { WorkspaceSidebar } from "./components/WorkspaceSidebar";
import { WorkspaceTopbar } from "./components/WorkspaceTopbar";
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
  ResourceSummary
} from "./types";
import {
  filterCategoryItems,
  type WorkspaceCategory
} from "./workspaceCategories";

type AdminSection = "users" | "groups" | "azure" | "connections" | "notifications" | "model" | "diagnostics";
type FormState =
  | { mode: "closed" }
  | { mode: "create"; draftType: ResourceForm["type"] }
  | { mode: "edit" };
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

// Events are stored forever; the UI pages through them AUDIT_PAGE_SIZE at a time.
const AUDIT_PAGE_SIZE = 100;

export default function App() {
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string>();
  const {
    loginOptions,
    setLoginOptions,
    session,
    setSession,
    booting,
    inviteToken,
    signIn,
    acceptInvite,
    changePassword,
    refreshCurrentSession,
    handleMicrosoftSignIn
  } = useAuth({ setBusy, setMessage });
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
  const [changePasswordOpen, setChangePasswordOpen] = useState(false);
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
    if (!user) {
      return;
    }
    void loadWorkspaceData();
  }, [user]);

  function signOut() {
    if (session) {
      void api.authLogout();
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
        loadAllResources(),
        loadActivity(),
        loadNotifications(),
        session.capabilities.canViewAudit ? loadAudit() : Promise.resolve(),
        session.capabilities.canViewAdmin
          ? Promise.all([
              loadAdminConfig(),
              loadArchivedResources(),
              loadLocalGroups(),
              loadKnownUsers(),
              loadNotificationDeliveries()
            ])
          : Promise.resolve()
      ]);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to load workspace");
    } finally {
      setBusy(false);
    }
  }

  async function loadAllResources(): Promise<ResourceSummary[]> {
    const response = await api.listResources(new URLSearchParams());
    setAllResources(response.items);
    return response.items;
  }

  async function loadResource(id: string) {
    try {
      const item = await api.getResource(id);
      setSelectedResource(item);
      setReveal(null);
      setLaunch(null);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to load resource");
    }
  }

  async function loadActivity() {
    const response = await api.myActivity();
    setActivity(response.items);
  }

  async function loadAudit(filters = auditFilters) {
    const response = await api.listAudit({
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
    const response = await api.listAudit({
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
      void loadAudit(filters);
    }
  }

  async function loadArchivedResources() {
    const response = await api.listArchivedResources();
    setArchivedResources(response.items);
  }

  // Invoked by useAdminUsers when refreshing the current user's own session
  // fails after an access change: clear everything App still owns (the hook
  // clears its own state before calling this).
  function onForcedSignOut(failureMessage: string) {
    // JS cannot drop the httpOnly cookie itself — invalidate the session
    // server-side, which also clears the cookie.
    void api.authLogout();
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
    setFormState(closedFormState);
    window.location.hash = "";
    setMessage(failureMessage);
  }

  const visibleCategories = (["connections", "keyvault", "appregistrations", "passwords"] as WorkspaceCategory[]).filter(
    (category) => session?.capabilities.categories[category]?.view
  );
  const categoryView: WorkspaceCategory | null = view === "activity" || view === "audit" || view === "admin" ? null : view;
  const categoryItems = categoryView ? filterCategoryItems(allResources, categoryView) : [];
  const currentItems = filterCatalogItems(categoryItems, filters);
  const archivedKeyVaultItems = archivedResources.filter((item) => item.type === "key_vault_secret");
  const currentArchivedKeyVaultItems = filterArchivedKeyVaultItems(archivedKeyVaultItems, filters);
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
      <WorkspaceSidebar view={view} visibleCategories={visibleCategories} capabilities={session.capabilities} />

      <main className="workspace-main">
        <WorkspaceTopbar
          view={view}
          currentUser={currentUser}
          canViewPasswords={session.capabilities.categories.passwords.view}
          showKeyVaultViewToggle={categoryView === "keyvault" && session.user.isAdmin}
          keyVaultViewMode={keyVaultViewMode}
          onKeyVaultViewModeChange={setKeyVaultViewMode}
          notifications={notifications}
          onMarkNotificationRead={handleMarkNotificationRead}
          onOpenNotificationResource={(resourceId) => {
            window.location.hash = "#appregistrations";
            setSelectedResourceId(resourceId);
          }}
          vaultUnlocked={vaultUnlocked}
          onToggleVaultLock={toggleVaultLock}
          onOpenBrowserExtensions={() => setBrowserExtensionManagerOpen(true)}
          onOpenChangePassword={() => setChangePasswordOpen(true)}
          onSignOut={signOut}
        />

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
                    // Drop any stale banner text so the modal starts clean; it
                    // mirrors the message state while open.
                    setMessage(undefined);
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
                  onEdit={() => {
                    setMessage(undefined);
                    setFormState({ mode: "edit" });
                  }}
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
            message={message}
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
            message={message}
            busy={busy}
            onRefresh={() => {
              if (session) {
                void loadKeyVaultDiscoveries();
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
            message={message}
            busy={busy}
            onRefresh={() => {
              if (session) {
                void loadAppRegistrationDiscoveries();
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
