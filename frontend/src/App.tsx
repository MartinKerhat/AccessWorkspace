import { useEffect, useRef, useState } from "react";
import { api, isVaultLocked } from "./api/client";
import { passkeysSupported, registerPasskey, unlockWithPasskey, isPrfUnavailable } from "./webauthn";
import { ResourceFormModal } from "./modals/ResourceFormModal";
import { AdminConfigModal } from "./modals/AdminConfigModal";
import { ChangePasswordModal } from "./modals/ChangePasswordModal";
import { VaultUnlockModal } from "./modals/VaultUnlockModal";
import { NotificationPolicyModal } from "./modals/NotificationPolicyModal";
import { KeyVaultSourcesModal } from "./modals/KeyVaultSourcesModal";
import { KeyVaultImportModal } from "./modals/KeyVaultImportModal";
import { AppRegistrationImportModal } from "./modals/AppRegistrationImportModal";
import { emptyKeyVaultSource, getSelectedKeyVaultItems } from "./keyVault";
import { getSelectedAppRegistrationItems } from "./appRegistration";
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
  AdminConfig,
  AdminForm,
  AppRegistrationDiscoverResult,
  AppRegistrationImportForm,
  AppRegistrationNotificationPolicy,
  AppRegistrationSyncResult,
  ArchivedResourceSummary,
  AuditEvent,
  AuthMode,
  BrowserExtensionClient,
  BrowserExtensionConnectState,
  BrowserExtensionRuntime,
  BrowserExtensionConnectToken,
  ConnectionCredentialOverride,
  CreateUserInput,
  KeyVaultDiscoverResult,
  KeyVaultImportItem,
  KeyVaultImportForm,
  KeyVaultSyncResult,
  LaunchPayload,
  LauncherRuntime,
  LocalGroup,
  LocalGroupForm,
  NotificationAdminForm,
  NotificationDeliveryRecord,
  NotificationPolicyModalState,
  Resource,
  ResourceForm,
  ResourceSummary,
  RevealResult,
  UserInvite,
  VaultStatus,
  UserNotification,
  VisibleResourceSummary,
  UserSummary,
  UserAccessDetail,
  UserAccessUpdateInput,
  User,
  WorkspaceCapabilities
} from "./types";
import {
  categoryLabel,
  filterCategoryItems,
  type WorkspaceCategory
} from "./workspaceCategories";

type View = WorkspaceCategory | "activity" | "audit" | "admin";
type AdminSection = "users" | "groups" | "azure" | "connections" | "notifications" | "model" | "diagnostics";
type Filters = { q: string; target: string };
type KeyVaultViewMode = "active" | "archived";
type Session = {
  user: User;
  authToken: string;
  authMode: AuthMode;
  capabilities: WorkspaceCapabilities;
};
type FormState =
  | { mode: "closed" }
  | { mode: "create"; draftType: ResourceForm["type"] }
  | { mode: "edit" };
type KeyVaultModalState =
  | { mode: "closed" }
  | { mode: "sources" }
  | { mode: "import" };
type AppRegistrationModalState =
  | { mode: "closed" }
  | { mode: "import" };
type LoginOptions = {
  localLoginEnabled: boolean;
  microsoftLoginHint: boolean;
};



const defaultFilters: Filters = { q: "", target: "" };
const authTokenStorageKey = "authToken";
const closedFormState: FormState = { mode: "closed" };
const closedKeyVaultModalState: KeyVaultModalState = { mode: "closed" };
const closedAppRegistrationModalState: AppRegistrationModalState = { mode: "closed" };
const availableRights = [
  "connections.read",
  "connections.edit",
  "keyvault.read",
  "keyvault.edit",
  "appregistrations.read",
  "appregistrations.edit",
  "passwords.read",
  "passwords.edit",
  "audit.read",
  "admin.access"
] as const;

function toAdminForm(config: AdminConfig | null): AdminForm {
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

function defaultAppRegistrationNotificationPolicy(): AppRegistrationNotificationPolicy {
  return {
    enabled: true,
    reminderDays: [30, 14, 7, 3, 1, 0],
    channels: ["in_app"]
  };
}

function toNotificationAdminForm(config: AdminConfig | null): NotificationAdminForm {
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

function detectBrowserExtensionClient(): BrowserExtensionClient {
  const userAgent = window.navigator.userAgent.toLowerCase();
  if (userAgent.includes("firefox")) {
    return "firefox";
  }
  if (userAgent.includes("safari") && !userAgent.includes("chrome") && !userAgent.includes("chromium") && !userAgent.includes("edg")) {
    return "safari";
  }
  if (userAgent.includes("chrome") || userAgent.includes("chromium") || userAgent.includes("edg")) {
    return "chromium";
  }
  return "unknown";
}

function connectInstalledBrowserExtension(connectToken: BrowserExtensionConnectToken) {
  const requestId = crypto.randomUUID();
  return new Promise<void>((resolve, reject) => {
    const timeout = window.setTimeout(() => {
      cleanup();
      reject(new Error("The extension did not respond. Install or reload it, refresh this page, and try again."));
    }, 3000);

    const cleanup = () => {
      window.clearTimeout(timeout);
      window.removeEventListener("message", handleMessage);
    };

    const handleMessage = (event: MessageEvent) => {
      if (event.source !== window) {
        return;
      }
      const data = event.data;
      if (!data || data.source !== "access-workspace-extension" || data.requestId !== requestId) {
        return;
      }
      cleanup();
      if (data.ok) {
        resolve();
        return;
      }
      reject(new Error(typeof data.error === "string" ? data.error : "Connecting the browser extension failed."));
    };

    window.addEventListener("message", handleMessage);
    window.postMessage({
      source: "access-workspace-webapp",
      type: "connect-browser-extension",
      requestId,
      payload: {
        workspaceBaseUrl: api.workspaceBaseUrl(),
        connectToken: connectToken.token
      }
    }, window.location.origin);
  });
}

// Events are stored forever; the UI pages through them AUDIT_PAGE_SIZE at a time.
const AUDIT_PAGE_SIZE = 100;

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

export default function App() {
  const currentBrowserClient = detectBrowserExtensionClient();
  const [loginOptions, setLoginOptions] = useState<LoginOptions>({
    localLoginEnabled: true,
    microsoftLoginHint: true
  });
  const [session, setSession] = useState<Session | null>(null);
  const [allResources, setAllResources] = useState<ResourceSummary[]>([]);
  const [selectedResourceId, setSelectedResourceId] = useState<string>();
  const [selectedResource, setSelectedResource] = useState<Resource>();
  const [passwordOptions, setPasswordOptions] = useState<ResourceSummary[]>([]);
  const [connectionOverride, setConnectionOverride] = useState<ConnectionCredentialOverride | null>(null);
  const [reveal, setReveal] = useState<RevealResult | null>(null);
  const [launch, setLaunch] = useState<LaunchPayload | null>(null);
  const [launcherRuntime, setLauncherRuntime] = useState<LauncherRuntime | null>(null);
  const [browserExtensionRuntime, setBrowserExtensionRuntime] = useState<BrowserExtensionRuntime | null>(null);
  const [browserExtensionManagerOpen, setBrowserExtensionManagerOpen] = useState(false);
  const [launcherDownloadsOpen, setLauncherDownloadsOpen] = useState(false);
  const [browserExtensionConnectState, setBrowserExtensionConnectState] = useState<BrowserExtensionConnectState | null>(null);
  const [activity, setActivity] = useState<AuditEvent[]>([]);
  const [notifications, setNotifications] = useState<UserNotification[]>([]);
  const [notificationDeliveries, setNotificationDeliveries] = useState<NotificationDeliveryRecord[]>([]);
  const [audit, setAudit] = useState<AuditEvent[]>([]);
  const [auditHasMore, setAuditHasMore] = useState(false);
  const [auditTotal, setAuditTotal] = useState(0);
  const [auditEventTypes, setAuditEventTypes] = useState<string[]>([]);
  const [auditFilters, setAuditFilters] = useState({ query: "", eventType: "" });
  const [adminConfig, setAdminConfig] = useState<AdminConfig | null>(null);
  const [archivedResources, setArchivedResources] = useState<ArchivedResourceSummary[]>([]);
  const [localGroups, setLocalGroups] = useState<LocalGroup[]>([]);
  const [knownUsers, setKnownUsers] = useState<UserSummary[]>([]);
  const [selectedAdminUserId, setSelectedAdminUserId] = useState<string>();
  const [selectedAdminUser, setSelectedAdminUser] = useState<UserAccessDetail>();
  const [selectedAdminUserResources, setSelectedAdminUserResources] = useState<VisibleResourceSummary[]>([]);
  const [adminForm, setAdminForm] = useState<AdminForm>(toAdminForm(null));
  const [keyVaultDiscoveries, setKeyVaultDiscoveries] = useState<KeyVaultDiscoverResult>({ sources: [] });
  const [keyVaultImportForm, setKeyVaultImportForm] = useState<KeyVaultImportForm>(emptyKeyVaultImportForm());
  const [appRegistrationDiscoveries, setAppRegistrationDiscoveries] = useState<AppRegistrationDiscoverResult>({ items: [] });
  const [appRegistrationImportForm, setAppRegistrationImportForm] = useState<AppRegistrationImportForm>(emptyAppRegistrationImportForm());
  const [keyVaultViewMode, setKeyVaultViewMode] = useState<KeyVaultViewMode>("active");
  const [selectedArchivedKeyVaultId, setSelectedArchivedKeyVaultId] = useState<string>();
  const [filters, setFilters] = useState<Filters>(defaultFilters);
  const [view, setView] = useState<View>(currentView());
  const [adminSection, setAdminSection] = useState<AdminSection>("users");
  const [busy, setBusy] = useState(false);
  const [booting, setBooting] = useState(true);
  const [message, setMessage] = useState<string>();
  const [revealCopyMessage, setRevealCopyMessage] = useState<string>();
  const [keyVaultSyncing, setKeyVaultSyncing] = useState(false);
  const [appRegistrationSyncing, setAppRegistrationSyncing] = useState(false);
  const [accountMenuOpen, setAccountMenuOpen] = useState(false);
  const [changePasswordOpen, setChangePasswordOpen] = useState(false);
  const [vaultPrompt, setVaultPrompt] = useState<{ status: VaultStatus; retry: () => Promise<void> } | null>(null);
  const [passkeyCapable, setPasskeyCapable] = useState(false);
  const [vaultUnlocked, setVaultUnlocked] = useState(false);
  const [inviteToken, setInviteToken] = useState<string>(() =>
    new URLSearchParams(window.location.search).get("invite") ?? ""
  );
  const [notificationCenterOpen, setNotificationCenterOpen] = useState(false);
  const [formState, setFormState] = useState<FormState>(closedFormState);
  const [adminModalOpen, setAdminModalOpen] = useState(false);
  const [keyVaultModalState, setKeyVaultModalState] = useState<KeyVaultModalState>(closedKeyVaultModalState);
  const [appRegistrationModalState, setAppRegistrationModalState] = useState<AppRegistrationModalState>(closedAppRegistrationModalState);
  const [notificationAdminForm, setNotificationAdminForm] = useState<NotificationAdminForm>(toNotificationAdminForm(null));
  const [notificationPolicyModalState, setNotificationPolicyModalState] = useState<NotificationPolicyModalState>({ mode: "closed" });
  const previousViewRef = useRef<View | null>(null);
  const previousAdminSectionRef = useRef<AdminSection | null>(null);
  const vaultCheckedTokenRef = useRef<string | null>(null);

  const user = session?.user ?? null;

  useEffect(() => {
    const onHashChange = () => setView(currentView());
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  useEffect(() => {
    void passkeysSupported().then(setPasskeyCapable);
  }, []);

  // After sign-in, surface the personal-passwords unlock once per session.
  // Local users arrive already unlocked (their login password derived the
  // key), so status.unlocked is true and nothing shows; SSO users have no
  // such key, so they're prompted to set up (first time) or unlock (Windows
  // Hello / passphrase). Runs once per session token.
  useEffect(() => {
    if (!session) {
      return;
    }
    if (vaultCheckedTokenRef.current === session.authToken) {
      return;
    }
    vaultCheckedTokenRef.current = session.authToken;
    void (async () => {
      try {
        const status = await api.vaultStatus(session.authToken);
        setVaultUnlocked(status.unlocked);
        if (!status.unlocked) {
          setVaultPrompt({ status, retry: async () => {} });
        }
      } catch {
        // Non-fatal: the reactive 423 path still prompts on first personal use.
      }
    })();
  }, [session]);

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

  useEffect(() => {
    let cancelled = false;

    async function loadLauncherRuntimeForSelection() {
      if (!selectedResource || (selectedResource.type !== "rdp" && selectedResource.type !== "ssh")) {
        return;
      }
      try {
        const runtime = await api.launcherRuntime();
        if (cancelled) {
          return;
        }
        setLauncherRuntime(runtime);
      } catch {
        if (!cancelled) {
          setLauncherRuntime(null);
        }
      }
    }

    void loadLauncherRuntimeForSelection();
    return () => {
      cancelled = true;
    };
  }, [selectedResource]);

  useEffect(() => {
    let cancelled = false;

    async function loadBrowserExtensionRuntime() {
      if (!session || !session.capabilities.categories.passwords.view) {
        setBrowserExtensionRuntime(null);
        return;
      }
      try {
        const runtime = await api.browserExtensionRuntime();
        if (cancelled) {
          return;
        }
        setBrowserExtensionRuntime(runtime);
      } catch {
        if (!cancelled) {
          setBrowserExtensionRuntime(null);
        }
      }
    }

    void loadBrowserExtensionRuntime();
    return () => {
      cancelled = true;
    };
  }, [session]);

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
          api.listPasswordOptions(session.authToken),
          api.getConnectionCredentialOverride(selectedResource.id, session.authToken)
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
    void loadResource(selectedResourceId, session.authToken);
  }, [selectedResourceId, session]);

  useEffect(() => {
    setAdminForm(toAdminForm(adminConfig));
    setNotificationAdminForm(toNotificationAdminForm(adminConfig));
  }, [adminConfig]);

  useEffect(() => {
    if (!session?.capabilities.canViewAdmin) {
      setSelectedAdminUserId(undefined);
      setSelectedAdminUser(undefined);
      setSelectedAdminUserResources([]);
      return;
    }
    if (!selectedAdminUserId && knownUsers.length > 0) {
      setSelectedAdminUserId(knownUsers[0].id);
      return;
    }
    if (selectedAdminUserId && !knownUsers.some((item) => item.id === selectedAdminUserId)) {
      setSelectedAdminUserId(knownUsers[0]?.id);
    }
  }, [knownUsers, selectedAdminUserId, session?.capabilities.canViewAdmin]);

  useEffect(() => {
    if (!session?.capabilities.canViewAdmin || !selectedAdminUserId) {
      setSelectedAdminUser(undefined);
      setSelectedAdminUserResources([]);
      return;
    }
    void loadAdminUserDetail(selectedAdminUserId, session.authToken);
  }, [selectedAdminUserId, session]);

  useEffect(() => {
    if (!reveal?.secretValue) {
      setRevealCopyMessage(undefined);
    }
  }, [reveal]);

  useEffect(() => {
    if (appRegistrationModalState.mode === "import") {
      return;
    }
  }, [appRegistrationModalState.mode]);

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
    setVaultUnlocked(false);
    setVaultPrompt(null);
    vaultCheckedTokenRef.current = null;
    setSession(null);
    setAllResources([]);
    setSelectedResourceId(undefined);
    setSelectedResource(undefined);
    setReveal(null);
    setLaunch(null);
    setActivity([]);
    setAudit([]);
    setAuditHasMore(false);
    setAuditTotal(0);
    setAuditEventTypes([]);
    setAuditFilters({ query: "", eventType: "" });
    setAdminConfig(null);
    setArchivedResources([]);
    setLocalGroups([]);
    setKnownUsers([]);
    setSelectedAdminUserId(undefined);
    setSelectedAdminUser(undefined);
    setSelectedAdminUserResources([]);
    setAdminForm(toAdminForm(null));
    setKeyVaultDiscoveries({ sources: [] });
    setKeyVaultImportForm(emptyKeyVaultImportForm());
    setAppRegistrationDiscoveries({ items: [] });
    setAppRegistrationImportForm(emptyAppRegistrationImportForm());
    setKeyVaultViewMode("active");
    setSelectedArchivedKeyVaultId(undefined);
    setFilters(defaultFilters);
    setView("connections");
    setMessage(undefined);
    setRevealCopyMessage(undefined);
    setKeyVaultSyncing(false);
    setAppRegistrationSyncing(false);
    setAccountMenuOpen(false);
    setFormState(closedFormState);
    setKeyVaultModalState(closedKeyVaultModalState);
    setAppRegistrationModalState(closedAppRegistrationModalState);
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

  async function loadNotifications(authToken: string) {
    const response = await api.myNotifications(authToken);
    setNotifications(response.items);
  }

  async function loadNotificationDeliveries(authToken: string) {
    const response = await api.listAdminNotificationDeliveries(authToken);
    setNotificationDeliveries(response.items);
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

  async function loadAdminConfig(authToken: string) {
    const response = await api.adminConfig(authToken);
    setAdminConfig(response);
    setLoginOptions((current) => ({
      ...current,
      microsoftLoginHint: response.entraEnabled && response.entraConfigured
    }));
  }

  async function loadArchivedResources(authToken: string) {
    const response = await api.listArchivedResources(authToken);
    setArchivedResources(response.items);
  }

  async function loadLocalGroups(authToken: string) {
    const response = await api.listLocalGroups(authToken);
    setLocalGroups(response.items);
  }

  async function loadKnownUsers(authToken: string) {
    const response = await api.listUsers(authToken);
    setKnownUsers(response.items);
  }

  async function loadAdminUserDetail(id: string, authToken: string) {
    const [userResponse, visibleResourcesResponse] = await Promise.all([
      api.getAdminUser(id, authToken),
      api.getAdminUserVisibleResources(id, authToken)
    ]);
    setSelectedAdminUser(userResponse);
    setSelectedAdminUserResources(visibleResourcesResponse.items);
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
          },
        session.authToken
      );
      setAdminConfig(response);
      setAdminForm(toAdminForm(response));
      setLoginOptions((current) => ({
        ...current,
        microsoftLoginHint: response.entraEnabled && response.entraConfigured
      }));
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
        },
        session.authToken
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
      await loadNotificationDeliveries(session.authToken);
      setMessage("Notification delivery log refreshed.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to refresh notification delivery log");
    } finally {
      setBusy(false);
    }
  }

  async function handleMarkNotificationRead(notificationID: string) {
    if (!session) {
      return;
    }
    try {
      await api.markNotificationRead(notificationID, session.authToken);
      setNotifications((current) =>
        current.map((item) => (item.id === notificationID ? { ...item, readAt: new Date().toISOString() } : item))
      );
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to update notification");
    }
  }


  async function handleSaveNotificationPolicyOverride() {
    if (!session || notificationPolicyModalState.mode !== "resource") {
      return;
    }
    setBusy(true);
    try {
      const resource = await api.updateAppRegistrationNotificationPolicies(
        notificationPolicyModalState.resource.id,
        {
          resourcePolicy: notificationPolicyModalState.useResourceOverride ? notificationPolicyModalState.draft : undefined,
          credentialPolicies: notificationPolicyModalState.credentialDrafts
        },
        session.authToken
      );
      setSelectedResource(resource);
      setAllResources((current) => current.map((item) => (item.id === resource.id ? { ...item, status: resource.status } : item)));
      setNotificationPolicyModalState({ mode: "closed" });
      await loadNotifications(session.authToken);
      setMessage("Notification policy updated.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to update notification policy");
    } finally {
      setBusy(false);
    }
  }

  async function handleSaveLocalGroup(mode: "create" | "edit", originalName: string | undefined, input: LocalGroupForm) {
    if (!session) {
      return;
    }
    setBusy(true);
    try {
      if (mode === "edit" && originalName) {
        await api.updateLocalGroup(originalName, input, session.authToken);
        setMessage("Local group updated");
      } else {
        await api.createLocalGroup(input, session.authToken);
        setMessage("Local group created");
      }
      await Promise.all([loadLocalGroups(session.authToken), loadKnownUsers(session.authToken)]);
      if (selectedAdminUserId) {
        await loadAdminUserDetail(selectedAdminUserId, session.authToken);
      }
      await refreshCurrentSession(session.authToken);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Saving local group failed");
      throw error;
    } finally {
      setBusy(false);
    }
  }

  async function loadKeyVaultDiscoveries(authToken: string) {
    const response = await api.discoverKeyVault(authToken);
    setKeyVaultDiscoveries({
      sources: (response.sources ?? []).map((source) => ({
        ...source,
        items: source.items ?? []
      }))
    });
  }

  async function loadAppRegistrationDiscoveries(authToken: string) {
    const response = await api.discoverAppRegistrations(authToken);
    setAppRegistrationDiscoveries({
      items: (response.items ?? []).map((item) => ({
        ...item,
        owners: item.owners ?? [],
        credentials: item.credentials ?? []
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
      const result = await api.syncKeyVault(automatic, session.authToken);
      if (session.user.isAdmin) {
        await Promise.all([loadAdminConfig(session.authToken), loadArchivedResources(session.authToken)]);
      }
      if (result.updatedResources > 0 || result.removedResources > 0 || result.missingResources > 0 || !automatic) {
        const items = await loadAllResources(session.authToken);
        if (selectedResourceId && items.some((item) => item.id === selectedResourceId)) {
          await loadResource(selectedResourceId, session.authToken);
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
        },
        session.authToken
      );
      setAdminConfig(response);
      setAdminForm(toAdminForm(response));
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
      await loadKeyVaultDiscoveries(session.authToken);
      setKeyVaultImportForm(emptyKeyVaultImportForm());
      setKeyVaultModalState({ mode: "import" });
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Key Vault discovery failed");
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
      await api.generateRDPSigningTestCertificate(session.authToken);
      const response = await api.adminConfig(session.authToken);
      setAdminConfig(response);
      setAdminForm(toAdminForm(response));
      setMessage("Test RDP signing certificate generated.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to generate test RDP signing certificate");
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
        },
        session.authToken
      );
      const createdItems = response.items ?? [];
      setMessage(createdItems.length === 1 ? "Key Vault secret imported" : `${createdItems.length} Key Vault secrets imported`);
      await loadAllResources(session.authToken);
      await loadActivity(session.authToken);
      if (session.capabilities.canViewAudit) {
        await loadAudit(session.authToken);
      }
      if (createdItems[0]) {
        setSelectedResourceId(createdItems[0].id);
        await loadResource(createdItems[0].id, session.authToken);
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
      const result = await api.syncAppRegistrations(automatic, session.authToken);
      if (result.updatedResources > 0 || result.removedResources > 0 || result.missingResources > 0 || !automatic) {
        const items = await loadAllResources(session.authToken);
        if (selectedResourceId && items.some((item) => item.id === selectedResourceId)) {
          await loadResource(selectedResourceId, session.authToken);
        } else if (selectedResourceId) {
          setSelectedResourceId(undefined);
          setSelectedResource(undefined);
          setReveal(null);
          setLaunch(null);
        }
      }
      if (session.capabilities.canViewAdmin && result.removedResources > 0) {
        await loadArchivedResources(session.authToken);
      }
      await loadNotifications(session.authToken);
      if (session.capabilities.canViewAdmin) {
        await loadAdminConfig(session.authToken);
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
      await loadAppRegistrationDiscoveries(session.authToken);
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
        },
        session.authToken
      );
      const createdItems = response.items ?? [];
      if (createdItems.length === 0) {
        setMessage("Selected app registrations were already imported");
      } else {
        setMessage(createdItems.length === 1 ? "App registration imported" : `${createdItems.length} app registrations imported`);
      }
      await loadAllResources(session.authToken);
      await loadActivity(session.authToken);
      if (session.capabilities.canViewAudit) {
        await loadAudit(session.authToken);
      }
      if (createdItems[0]) {
        setSelectedResourceId(createdItems[0].id);
        await loadResource(createdItems[0].id, session.authToken);
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

  async function refreshAfterSensitiveAction() {
    if (!session) {
      return;
    }
    await loadActivity(session.authToken);
    if (session.capabilities.canViewAudit) {
      await loadAudit(session.authToken);
    }
  }

  // Personal secrets need the vault unlocked in this session. On a 423 the
  // action is parked, the unlock/setup modal opens, and the same action is
  // retried automatically after a successful unlock.
  async function guardVaultLocked(error: unknown, retry: () => Promise<void>): Promise<boolean> {
    if (!isVaultLocked(error) || !session) {
      return false;
    }
    try {
      const status = await api.vaultStatus(session.authToken);
      setVaultPrompt({ status, retry });
    } catch {
      setVaultPrompt({ status: { hasVault: true, unlocked: false, methods: [], passkeys: [] }, retry });
    }
    return true;
  }

  // Account-menu entry point: when locked, open the setup/unlock prompt; when
  // unlocked, re-lock this session (the meaningful action once unlocked, so
  // the button is never a dead end).
  async function toggleVaultLock() {
    if (!session) {
      return;
    }
    try {
      const status = await api.vaultStatus(session.authToken);
      if (status.unlocked) {
        await api.vaultLock(session.authToken);
        setVaultUnlocked(false);
        setMessage("Personal passwords locked");
        return;
      }
      setVaultPrompt({ status, retry: async () => {} });
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Checking the vault failed");
    }
  }

  async function afterVaultUnlocked(): Promise<boolean> {
    const retry = vaultPrompt?.retry;
    setVaultPrompt(null);
    setVaultUnlocked(true);
    if (retry) {
      await retry();
    }
    return true;
  }

  async function submitVaultPassphrase(passphrase: string): Promise<boolean> {
    if (!session || !vaultPrompt) {
      return false;
    }
    setBusy(true);
    try {
      if (vaultPrompt.status.hasVault) {
        await api.vaultUnlock(passphrase, session.authToken);
      } else {
        await api.vaultSetup(passphrase, session.authToken);
      }
      return await afterVaultUnlocked();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unlocking the vault failed");
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function submitVaultPasskey(): Promise<boolean> {
    if (!session || !vaultPrompt) {
      return false;
    }
    setBusy(true);
    try {
      if (vaultPrompt.status.hasVault) {
        const unlock = await unlockWithPasskey(vaultPrompt.status.passkeys);
        await api.vaultPasskeyUnlock(unlock, session.authToken);
      } else {
        const registration = await registerPasskey(session.user.id, session.user.name);
        await api.vaultPasskeySetup(registration, session.authToken);
      }
      return await afterVaultUnlocked();
    } catch (error) {
      if (isPrfUnavailable(error)) {
        setMessage("This device can't use Windows Hello for the vault. Use a passphrase instead.");
      } else {
        setMessage(error instanceof Error ? error.message : "Windows Hello failed");
      }
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function handleReveal() {
    if (!selectedResourceId || !session) {
      return undefined;
    }
    setBusy(true);
    try {
      const response = await api.revealResource(selectedResourceId, session.authToken);
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
      const response = await api.revealResource(selectedResourceId, session.authToken);
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

  async function refreshLauncherStatus(runtimeArg?: LauncherRuntime | null) {
    const runtime = runtimeArg ?? launcherRuntime;
    if (!runtime) {
      return null;
    }
    try {
      return await api.launcherLocalStatus(runtime.statusUrl);
    } catch {
      return null;
    }
  }

  async function handleLaunch() {
    if (!selectedResourceId || !session) {
      return;
    }
    setBusy(true);
    try {
      const response = await api.launchResource(selectedResourceId, session.authToken);
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
        setMessage("Launch target prepared.");
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

  async function handlePrepareBrowserExtensionSession() {
    if (!session) {
      return;
    }
    setMessage(undefined);
    setBusy(true);
    try {
      const connectToken = await api.createBrowserExtensionConnectToken(session.authToken);
      setBrowserExtensionConnectState({
        user: connectToken.user,
        phase: "connecting"
      });
      await connectInstalledBrowserExtension(connectToken);
      setBrowserExtensionConnectState({
        user: connectToken.user,
        phase: "connected"
      });
    } catch (error) {
      const fallbackMessage = error instanceof Error ? error.message : "Connecting the browser extension failed";
      setBrowserExtensionConnectState({
        user: session.user,
        phase: "unavailable",
        error: fallbackMessage
      });
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
      const override = await api.setConnectionCredentialOverride(selectedResourceId, passwordResourceId, session.authToken);
      setConnectionOverride(override);
      setMessage("Personal connection override saved.");
      if (session.capabilities.canViewAudit) {
        await loadAudit(session.authToken);
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
      await api.clearConnectionCredentialOverride(selectedResourceId, session.authToken);
      setConnectionOverride({
        connectionId: selectedResourceId,
        passwordResourceId: "",
        passwordResourceName: "",
        username: "",
        personal: false
      });
      setMessage("Personal connection override cleared.");
      if (session.capabilities.canViewAudit) {
        await loadAudit(session.authToken);
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
      const created = await api.createResource(input, session.authToken);
      setMessage("Object created");
      await loadAllResources(session.authToken);
      setSelectedResourceId(created.id);
      await loadResource(created.id, session.authToken);
      if (session.capabilities.canViewAudit) {
        await loadAudit(session.authToken);
      }
      setFormState(closedFormState);
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
      await api.updateResource(selectedResourceId, input, session.authToken);
      setMessage("Object updated");
      await loadAllResources(session.authToken);
      await loadResource(selectedResourceId, session.authToken);
      if (session.capabilities.canViewAudit) {
        await loadAudit(session.authToken);
      }
      setFormState(closedFormState);
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
    const canManageOwnedResource =
      selectedResource.ownerUserId === session.user.id &&
      session.capabilities.categories[selectedResource.category]?.edit;
    if (!session.user.isAdmin && !canManageOwnedResource) {
      setMessage("You can only remove objects you own.");
      return;
    }
    const confirmed = window.confirm(
      selectedResource.type === "key_vault_secret"
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
      await api.archiveResource(selectedResourceId, session.authToken);
      setMessage("Object removed from app");
      await loadAllResources(session.authToken);
      if (session.capabilities.canViewAdmin) {
        await loadArchivedResources(session.authToken);
      }
      if (session.capabilities.canViewAudit) {
        await loadAudit(session.authToken);
      }
      setSelectedResourceId(undefined);
      setSelectedResource(undefined);
      setReveal(null);
      setLaunch(null);
      setFormState(closedFormState);
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
      await api.restoreArchivedResource(item.id, session.authToken);
      setMessage(`${item.name} restored to the workspace catalog`);
      await Promise.all([loadAllResources(session.authToken), loadArchivedResources(session.authToken)]);
      if (session.capabilities.canViewAudit) {
        await loadAudit(session.authToken);
      }
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Restore failed");
    } finally {
      setBusy(false);
    }
  }

  async function handleSaveAdminUserAccess(input: UserAccessUpdateInput) {
    if (!session || !selectedAdminUserId) {
      return;
    }
    setBusy(true);
    try {
      const updated = await api.updateAdminUser(selectedAdminUserId, input, session.authToken);
      setSelectedAdminUser(updated);
      await Promise.all([loadKnownUsers(session.authToken), loadLocalGroups(session.authToken)]);
      if (session.capabilities.canViewAudit) {
        await loadAudit(session.authToken);
      }
      if (selectedAdminUserId === session.user.id) {
        try {
          await refreshCurrentSession(session.authToken);
        } catch (error) {
          localStorage.removeItem(authTokenStorageKey);
          setSession(null);
          setAllResources([]);
          setSelectedResourceId(undefined);
          setSelectedResource(undefined);
          setReveal(null);
          setLaunch(null);
          setActivity([]);
          setAudit([]);
          setAdminConfig(null);
          setArchivedResources([]);
          setLocalGroups([]);
          setKnownUsers([]);
          setSelectedAdminUserId(undefined);
          setSelectedAdminUser(undefined);
          setSelectedAdminUserResources([]);
          setAdminForm(toAdminForm(null));
          setKeyVaultDiscoveries({ sources: [] });
          setKeyVaultImportForm(emptyKeyVaultImportForm());
          setAppRegistrationDiscoveries({ items: [] });
          setAppRegistrationImportForm(emptyAppRegistrationImportForm());
          setKeyVaultViewMode("active");
          setSelectedArchivedKeyVaultId(undefined);
          setFilters(defaultFilters);
          setView("connections");
          setRevealCopyMessage(undefined);
          setKeyVaultSyncing(false);
          setAppRegistrationSyncing(false);
          setAccountMenuOpen(false);
          setFormState(closedFormState);
          setKeyVaultModalState(closedKeyVaultModalState);
          setAppRegistrationModalState(closedAppRegistrationModalState);
          window.location.hash = "";
          setMessage(error instanceof Error ? error.message : "Session refresh failed after updating user access");
          return;
        }
      }
      setMessage("User access updated");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Saving user access failed");
    } finally {
      setBusy(false);
    }
  }

  async function handleCreateAdminUser(input: CreateUserInput): Promise<{ ok: boolean; invite: UserInvite | null }> {
    if (!session) {
      return { ok: false, invite: null };
    }
    setBusy(true);
    try {
      const { user: created, invite } = await api.createAdminUser(input, session.authToken);
      setSelectedAdminUserId(created.id);
      setSelectedAdminUser(created);
      await Promise.all([loadKnownUsers(session.authToken), loadLocalGroups(session.authToken)]);
      await loadAdminUserDetail(created.id, session.authToken);
      if (session.capabilities.canViewAudit) {
        await loadAudit(session.authToken);
      }
      setMessage(invite ? `User ${created.name} created — share the invite link` : `User ${created.name} created`);
      return { ok: true, invite };
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Creating user failed");
      return { ok: false, invite: null };
    } finally {
      setBusy(false);
    }
  }

  async function handleResetAdminUserPassword(target: UserAccessDetail): Promise<UserInvite | null> {
    if (!session) {
      return null;
    }
    setBusy(true);
    try {
      const invite = await api.resetUserPassword(target.id, session.authToken);
      await loadKnownUsers(session.authToken);
      await loadAdminUserDetail(target.id, session.authToken);
      if (session.capabilities.canViewAudit) {
        await loadAudit(session.authToken);
      }
      setMessage(
        invite.personalResourcesDeleted && invite.personalResourcesDeleted > 0
          ? `Password reset for ${target.name} — ${invite.personalResourcesDeleted} personal password(s) were destroyed`
          : `Password reset for ${target.name} — share the reset link`
      );
      return invite;
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Resetting password failed");
      return null;
    } finally {
      setBusy(false);
    }
  }

  async function handleDeleteAdminUser(target: UserAccessDetail) {
    if (!session) {
      return;
    }
    const confirmed = window.confirm(
      `Delete ${target.name}? This permanently removes the account and deletes all of their personal saved passwords. Their shared objects are kept and reassigned to you. This cannot be undone.`
    );
    if (!confirmed) {
      return;
    }
    setBusy(true);
    try {
      const result = await api.deleteAdminUser(target.id, session.authToken);
      if (selectedAdminUserId === target.id) {
        setSelectedAdminUserId(undefined);
        setSelectedAdminUser(undefined);
        setSelectedAdminUserResources([]);
      }
      await Promise.all([
        loadKnownUsers(session.authToken),
        loadLocalGroups(session.authToken),
        loadAllResources(session.authToken)
      ]);
      if (session.capabilities.canViewAudit) {
        await loadAudit(session.authToken);
      }
      setMessage(
        `User ${target.name} deleted — ${result.personalResourcesDeleted} personal ${
          result.personalResourcesDeleted === 1 ? "object" : "objects"
        } removed, ${result.sharedResourcesReassigned} shared ${
          result.sharedResourcesReassigned === 1 ? "object" : "objects"
        } reassigned to you`
      );
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Deleting user failed");
    } finally {
      setBusy(false);
    }
  }

  const visibleCategories = (["connections", "keyvault", "appregistrations", "passwords"] as WorkspaceCategory[]).filter(
    (category) => session?.capabilities.categories[category]?.view
  );
  const categoryView: WorkspaceCategory | null = view === "activity" || view === "audit" || view === "admin" ? null : view;
  const categoryItems = categoryView ? filterCategoryItems(allResources, categoryView) : [];
  const currentItems = categoryItems.filter((item) => {
    const query = filters.q.trim().toLowerCase();
    const target = filters.target.trim().toLowerCase();

    const matchesQuery =
      query === "" ||
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
  const importedAppRegistrationIds = new Set(
    allResources
      .filter((item) => item.type === "app_registration")
      .flatMap((item) => [item.applicationId])
      .filter(Boolean)
  );

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
    if (category === "passwords") {
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
  const visibleBrowserPackages = browserExtensionRuntime
    ? browserExtensionRuntime.packages.filter((item) => currentUser.isAdmin || item.id !== "extension-firefox-unsigned")
    : [];
  const currentBrowserPackage = browserExtensionRuntime
    ? visibleBrowserPackages.find((item) => item.browser === currentBrowserClient) ?? null
    : null;

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
            <div className="account-menu">
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
            <div className="account-menu">
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
                      session.capabilities.categories[selectedResource.category]?.edit &&
                      ((!selectedResource.personal && session.user.isAdmin) ||
                        selectedResource.ownerUserId === session.user.id)
                  )}
                  canRemove={Boolean(
                    selectedResource &&
                    session.capabilities.categories[selectedResource.category]?.edit &&
                    ((!selectedResource.personal && session.user.isAdmin) ||
                      selectedResource.ownerUserId === session.user.id)
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
            restrictPasswordToPersonal={
              !session.user.isAdmin &&
              formState.mode === "create" &&
              formState.draftType !== undefined &&
              categoryView === "passwords"
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
                (selectedResource.ownerUserId === session.user.id &&
                  session.capabilities.categories[selectedResource.category]?.edit))
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
            onClose={() => {
              setReveal(null);
              setRevealCopyMessage(undefined);
            }}
            onCopy={() => void handleCopyRevealSecret()}
          />
        ) : null}
      </main>
    </div>
  );
}
