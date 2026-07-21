import type {
  AdminConfig,
  AppRegistrationDiscoverResult,
  AppRegistrationImportForm,
  AppRegistrationSyncResult,
  ArchivedResourceSummary,
  AuthMode,
  AuditEvent,
  NotificationDeliveryRecord,
  KeyVaultDiscoverResult,
  KeyVaultImportItem,
  KeyVaultImportForm,
  KeyVaultSyncResult,
  WorkspaceCapabilities,
  LaunchPayload,
  LauncherLocalStatus,
  LauncherRuntime,
  LocalGroup,
  LocalGroupForm,
  Resource,
  ResourceForm,
  ResourceSummary,
  RevealResult,
  AppRegistrationCredentialPolicyInput,
  AppRegistrationNotificationPolicy,
  BrowserExtensionRuntime,
  BrowserExtensionConnectToken,
  ConnectionCredentialOverride,
  CreateUserInput,
  UserInvite,
  VaultStatus,
  UserNotification,
  VisibleResourceSummary,
  UserSummary,
  UserAccessDetail,
  UserAccessUpdateInput,
  User
} from "../types";

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080/api";

function baseUrlFromApiBase() {
  const trimmed = API_BASE.replace(/\/+$/, "").replace(/\/api$/, "");
  // A relative API base (prod, served same-origin behind the ingress) collapses
  // to "" here. The browser extension runs in its own context and needs an
  // absolute origin, so resolve it against the current page.
  if (trimmed === "" || trimmed.startsWith("/")) {
    return window.location.origin;
  }
  return trimmed;
}

// absoluteApiBase returns API_BASE as an absolute URL. Same-origin fetches from
// the page use API_BASE directly, but URLs we hand to *external* agents (the
// desktop launcher, the browser extension) must be absolute — a relative "/api"
// is meaningless in their context.
function absoluteApiBase() {
  const trimmed = API_BASE.replace(/\/+$/, "");
  if (/^https?:\/\//i.test(trimmed)) {
    return trimmed;
  }
  return `${window.location.origin}${trimmed.startsWith("/") ? "" : "/"}${trimmed}`;
}

// Artifact download URLs come from the backend as root-relative paths
// ("/api/artifacts/download/..."). Prefix the API origin so they resolve both
// same-origin (prod, behind the ingress) and cross-origin (dev, API on :8080).
function absolutizeArtifactUrl(url: string): string {
  if (!url || /^https?:\/\//i.test(url)) {
    return url;
  }
  return `${baseUrlFromApiBase()}${url}`;
}

type AuthBootstrapResponse = {
  authMode: AuthMode;
  localLoginEnabled: boolean;
  microsoftLoginHint: boolean;
};

type AuthMeResponse = {
  user: User;
  authMode: AuthMode;
  capabilities: WorkspaceCapabilities;
};

// The session token never reaches page JavaScript — it travels in the
// httpOnly cookie the backend sets on login/invite/SSO.
type AuthLoginResponse = {
  user: User;
  authMode: AuthMode;
  capabilities: WorkspaceCapabilities;
};

export class ApiError extends Error {
  status: number;
  code?: string;
  constructor(message: string, status: number, code?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

export function isVaultLocked(error: unknown): boolean {
  return error instanceof ApiError && error.code === "vault_locked";
}

// The session rides in the httpOnly cookie; credentials mode makes the
// browser attach it (needed in dev, where the SPA is a different origin).
// `bearerToken` exists solely for the one-time localStorage migration.
async function request<T>(path: string, options: RequestInit = {}, bearerToken?: string): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...(bearerToken ? { Authorization: `Bearer ${bearerToken}` } : {}),
      ...(options.headers ?? {})
    }
  });

  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as { error?: string; code?: string } | null;
    throw new ApiError(body?.error ?? `Request failed with status ${response.status}`, response.status, body?.code);
  }

  return response.json() as Promise<T>;
}

export const api = {
  workspaceBaseUrl() {
    return baseUrlFromApiBase();
  },
  microsoftStartUrl() {
    return `${API_BASE}/auth/microsoft/start`;
  },
  authBootstrap() {
    return request<AuthBootstrapResponse>("/auth/bootstrap");
  },
  authLogin(username: string, password: string) {
    return request<AuthLoginResponse>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password })
    });
  },
  authMe() {
    return request<AuthMeResponse>("/auth/me");
  },
  // One-time migration: a token still sitting in localStorage from before the
  // cookie era is sent as a bearer once and comes back as the httpOnly cookie.
  upgradeSessionCookie(legacyToken: string) {
    return request<{ status: string }>("/auth/session/cookie", { method: "POST" }, legacyToken);
  },
  acceptInvite(token: string, password: string) {
    return request<AuthLoginResponse>("/auth/invite/accept", {
      method: "POST",
      body: JSON.stringify({ token, password })
    });
  },
  vaultStatus() {
    return request<VaultStatus>("/auth/vault");
  },
  vaultSetup(passphrase: string) {
    return request<{ status: string }>("/auth/vault/setup", { method: "POST", body: JSON.stringify({ passphrase }) });
  },
  vaultUnlock(passphrase: string) {
    return request<{ status: string }>("/auth/vault/unlock", { method: "POST", body: JSON.stringify({ passphrase }) });
  },
  vaultPasskeySetup(payload: { credentialId: string; prfSalt: string; prfSecret: string }) {
    return request<{ status: string }>("/auth/vault/passkey/setup", { method: "POST", body: JSON.stringify(payload) });
  },
  vaultPasskeyUnlock(payload: { credentialId: string; prfSecret: string }) {
    return request<{ status: string }>("/auth/vault/passkey/unlock", { method: "POST", body: JSON.stringify(payload) });
  },
  vaultLock() {
    return request<{ status: string }>("/auth/vault/lock", { method: "POST" });
  },
  changePassword(currentPassword: string, newPassword: string) {
    return request<{ status: string }>("/auth/password", {
      method: "POST",
      body: JSON.stringify({ currentPassword, newPassword })
    });
  },
  issueUserInvite(userId: string) {
    return request<UserInvite>(`/admin/users/${encodeURIComponent(userId)}/invite`, { method: "POST" });
  },
  resetUserPassword(userId: string) {
    return request<UserInvite>(`/admin/users/${encodeURIComponent(userId)}/reset-password`, { method: "POST" });
  },
  authLogout() {
    return request<{ status: string }>("/auth/logout", { method: "POST" });
  },
  listResources(params: URLSearchParams) {
    return request<{ items: ResourceSummary[] }>(`/resources?${params.toString()}`);
  },
  getResource(id: string) {
    return request<Resource>(`/resources/${id}`);
  },
  createResource(input: ResourceForm) {
    return request<Resource>("/resources", {
      method: "POST",
      body: JSON.stringify(input)
    });
  },
  updateResource(id: string, input: ResourceForm) {
    return request<Resource>(`/resources/${id}`, {
      method: "PUT",
      body: JSON.stringify(input)
    });
  },
  archiveResource(id: string) {
    return request<{ status: string }>(`/resources/${id}/archive`, {
      method: "POST"
    });
  },
  revealResource(id: string) {
    return request<RevealResult>(`/resources/${id}/reveal`, {
      method: "POST"
    });
  },
  launchResource(id: string) {
    return request<LaunchPayload>(`/resources/${id}/launch`, {
      method: "POST"
    });
  },
  launcherTicketResolveUrl(ticket: string) {
    // Handed to the desktop launcher, which fetches it from its own process —
    // must be absolute, not the page-relative "/api".
    return `${absoluteApiBase()}/launcher/tickets/${encodeURIComponent(ticket)}`;
  },
  listPasswordOptions() {
    return request<{ items: ResourceSummary[] }>("/passwords/options");
  },
  getConnectionCredentialOverride(id: string) {
    return request<ConnectionCredentialOverride>(`/resources/${id}/connection-override`);
  },
  setConnectionCredentialOverride(id: string, passwordResourceId: string) {
    return request<ConnectionCredentialOverride>(`/resources/${id}/connection-override`, {
      method: "PUT",
      body: JSON.stringify({ passwordResourceId })
    });
  },
  clearConnectionCredentialOverride(id: string) {
    return request<{ status: string }>(`/resources/${id}/connection-override`, {
      method: "DELETE"
    });
  },
  async launcherRuntime() {
    const runtime = await request<LauncherRuntime>("/launcher/runtime");
    return {
      ...runtime,
      downloadUrl: absolutizeArtifactUrl(runtime.downloadUrl),
      downloads: (runtime.downloads ?? []).map((file) => ({
        ...file,
        downloadUrl: absolutizeArtifactUrl(file.downloadUrl)
      }))
    };
  },
  async browserExtensionRuntime() {
    const runtime = await request<BrowserExtensionRuntime>("/browser-extension/runtime");
    return {
      ...runtime,
      downloadUrl: absolutizeArtifactUrl(runtime.downloadUrl),
      packages: (runtime.packages ?? []).map((pkg) => ({
        ...pkg,
        downloadUrl: pkg.downloadUrl ? absolutizeArtifactUrl(pkg.downloadUrl) : pkg.downloadUrl,
        files: (pkg.files ?? []).map((file) => ({
          ...file,
          downloadUrl: absolutizeArtifactUrl(file.downloadUrl)
        }))
      }))
    };
  },
  createBrowserExtensionConnectToken() {
    return request<BrowserExtensionConnectToken>("/auth/browser-extension-connect-token", {
      method: "POST"
    });
  },
  async launcherLocalStatus(statusUrl: string) {
    const response = await fetch(statusUrl);
    if (!response.ok) {
      throw new Error(`Launcher status failed with status ${response.status}`);
    }
    return response.json() as Promise<LauncherLocalStatus>;
  },
  async launcherLocalLaunch(launchUrl: string, payload: LaunchPayload) {
    const response = await fetch(launchUrl, {
      method: "POST",
      headers: {
        "Content-Type": "application/json"
      },
      body: JSON.stringify(payload)
    });
    if (!response.ok) {
      const body = (await response.json().catch(() => null)) as { error?: string } | null;
      throw new Error(body?.error ?? `Launcher launch failed with status ${response.status}`);
    }
    return response.json() as Promise<{ status: string }>;
  },
  listAudit(options: { limit?: number; offset?: number; query?: string; eventType?: string } = {}) {
    const params = new URLSearchParams();
    params.set("limit", String(options.limit ?? 100));
    params.set("offset", String(options.offset ?? 0));
    if (options.query?.trim()) {
      params.set("q", options.query.trim());
    }
    if (options.eventType) {
      params.set("eventType", options.eventType);
    }
    return request<{ items: AuditEvent[]; total: number; eventTypes: string[] }>(`/audit?${params.toString()}`);
  },
  adminConfig() {
    return request<AdminConfig>("/admin/config");
  },
  updateAdminConfig(input: {
    entraTenantId?: string;
    entraClientId?: string;
    entraAuthority?: string;
    entraRedirectUri?: string;
    entraGroupSource?: string;
    entraEnabled?: boolean;
    azureReaderUseAmbientIdentity?: boolean;
    keyVaultSources?: AdminConfig["keyVaultSources"];
    appRegistrationNotificationPolicy?: AdminConfig["appRegistrationNotificationPolicy"];
    notificationEmailEnabled?: boolean;
    notificationEmailHost?: string;
    notificationEmailPort?: number;
    notificationEmailUsername?: string;
    notificationEmailFrom?: string;
    appRegistrationAutoSyncEnabled?: boolean;
    appRegistrationSyncIntervalMinutes?: number;
    entraClientSecret?: string;
    notificationEmailPassword?: string;
    rdpSigningEnabled?: boolean;
  }) {
    return request<AdminConfig>("/admin/config", {
      method: "PUT",
      body: JSON.stringify(input)
    });
  },
  generateRDPSigningTestCertificate() {
    return request<{ enabled: boolean; certificateConfigured: boolean; subject: string; thumbprintSha256: string; generatedAt?: string }>(
      "/admin/rdp-signing/test-certificate",
      {
        method: "POST"
      }
    );
  },
  myActivity() {
    return request<{ items: AuditEvent[] }>("/me/activity");
  },
  myNotifications() {
    return request<{ items: UserNotification[] }>("/me/notifications");
  },
  markNotificationRead(id: string) {
    return request<{ status: string }>(`/me/notifications/${encodeURIComponent(id)}/read`, { method: "POST" });
  },
  listLocalGroups() {
    return request<{ items: LocalGroup[] }>("/admin/local-groups");
  },
  listArchivedResources() {
    return request<{ items: ArchivedResourceSummary[] }>("/admin/archived-resources");
  },
  listUsers() {
    return request<{ items: UserSummary[] }>("/admin/users");
  },
  async createAdminUser(input: CreateUserInput) {
    // Invite mode responds with { user, invite }; direct mode with the user.
    const response = await request<UserAccessDetail | { user: UserAccessDetail; invite: UserInvite }>("/admin/users", {
      method: "POST",
      body: JSON.stringify(input)
    });
    if ("invite" in response) {
      return response;
    }
    return { user: response, invite: null as UserInvite | null };
  },
  listAdminNotificationDeliveries() {
    return request<{ items: NotificationDeliveryRecord[] }>("/admin/notification-deliveries");
  },
  getAdminUser(id: string) {
    return request<UserAccessDetail>(`/admin/users/${encodeURIComponent(id)}`);
  },
  getAdminUserVisibleResources(id: string) {
    return request<{ items: VisibleResourceSummary[] }>(`/admin/users/${encodeURIComponent(id)}/visible-resources`);
  },
  updateAdminUser(id: string, input: UserAccessUpdateInput) {
    return request<UserAccessDetail>(`/admin/users/${encodeURIComponent(id)}`, {
      method: "PUT",
      body: JSON.stringify(input)
    });
  },
  deleteAdminUser(id: string) {
    return request<{ personalResourcesDeleted: number; sharedResourcesReassigned: number }>(
      `/admin/users/${encodeURIComponent(id)}`,
      {
        method: "DELETE"
      }
    );
  },
  restoreArchivedResource(id: string) {
    return request<{ status: string }>(`/admin/archived-resources/${id}/restore`, {
      method: "POST"
    });
  },
  discoverKeyVault() {
    return request<KeyVaultDiscoverResult>("/keyvault/discover");
  },
  importKeyVaultSecrets(input: KeyVaultImportForm & { items: KeyVaultImportItem[] }) {
    return request<{ items: Resource[] }>("/keyvault/import", {
      method: "POST",
      body: JSON.stringify(input)
    });
  },
  syncKeyVault(automatic: boolean) {
    return request<KeyVaultSyncResult>("/keyvault/sync", {
      method: "POST",
      body: JSON.stringify({ automatic })
    });
  },
  discoverAppRegistrations() {
    return request<AppRegistrationDiscoverResult>("/appregistrations/discover");
  },
  importAppRegistrations(input: AppRegistrationImportForm & { tenantId: string }) {
    return request<{ items: Resource[] }>("/appregistrations/import", {
      method: "POST",
      body: JSON.stringify({
        owner: input.owner,
        ownerTeam: input.ownerTeam,
        environment: input.environment,
        tenantId: input.tenantId,
        description: input.description,
        notes: input.notes,
        allowedGroups: input.allowedGroups,
        applicationIds: input.selectedApplicationIds
      })
    });
  },
  syncAppRegistrations(automatic: boolean) {
    return request<AppRegistrationSyncResult>("/appregistrations/sync", {
      method: "POST",
      body: JSON.stringify({ automatic })
    });
  },
  updateAppRegistrationNotificationPolicies(
    id: string,
    input: { resourcePolicy?: AppRegistrationNotificationPolicy; credentialPolicies: AppRegistrationCredentialPolicyInput[] }
  ) {
    return request<Resource>(`/resources/${encodeURIComponent(id)}/app-registration-notifications`, {
      method: "PUT",
      body: JSON.stringify(input)
    });
  },
  createLocalGroup(input: LocalGroupForm) {
    return request<{ status: string }>("/admin/local-groups", {
      method: "POST",
      body: JSON.stringify(input)
    });
  },
  updateLocalGroup(name: string, input: LocalGroupForm) {
    return request<{ status: string }>(`/admin/local-groups/${encodeURIComponent(name)}`, {
      method: "PUT",
      body: JSON.stringify(input)
    });
  }
};
