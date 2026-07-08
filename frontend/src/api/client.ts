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

type AuthLoginResponse = {
  token: string;
  user: User;
  authMode: AuthMode;
  capabilities: WorkspaceCapabilities;
};

async function request<T>(path: string, options: RequestInit = {}, authToken?: string): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(authToken ? { Authorization: `Bearer ${authToken}` } : {}),
      ...(options.headers ?? {})
    }
  });

  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as { error?: string } | null;
    throw new Error(body?.error ?? `Request failed with status ${response.status}`);
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
  authMe(authToken: string) {
    return request<AuthMeResponse>("/auth/me", {}, authToken);
  },
  authLogout(authToken: string) {
    return request<{ status: string }>("/auth/logout", { method: "POST" }, authToken);
  },
  listResources(params: URLSearchParams, authToken: string) {
    return request<{ items: ResourceSummary[] }>(`/resources?${params.toString()}`, {}, authToken);
  },
  getResource(id: string, authToken: string) {
    return request<Resource>(`/resources/${id}`, {}, authToken);
  },
  createResource(input: ResourceForm, authToken: string) {
    return request<Resource>(
      "/resources",
      {
        method: "POST",
        body: JSON.stringify(input)
      },
      authToken
    );
  },
  updateResource(id: string, input: ResourceForm, authToken: string) {
    return request<Resource>(
      `/resources/${id}`,
      {
        method: "PUT",
        body: JSON.stringify(input)
      },
      authToken
    );
  },
  archiveResource(id: string, authToken: string) {
    return request<{ status: string }>(
      `/resources/${id}/archive`,
      {
        method: "POST"
      },
      authToken
    );
  },
  revealResource(id: string, authToken: string) {
    return request<RevealResult>(
      `/resources/${id}/reveal`,
      {
        method: "POST"
      },
      authToken
    );
  },
  launchResource(id: string, authToken: string) {
    return request<LaunchPayload>(
      `/resources/${id}/launch`,
      {
        method: "POST"
      },
      authToken
    );
  },
  launcherTicketResolveUrl(ticket: string) {
    // Handed to the desktop launcher, which fetches it from its own process —
    // must be absolute, not the page-relative "/api".
    return `${absoluteApiBase()}/launcher/tickets/${encodeURIComponent(ticket)}`;
  },
  listPasswordOptions(authToken: string) {
    return request<{ items: ResourceSummary[] }>("/passwords/options", {}, authToken);
  },
  getConnectionCredentialOverride(id: string, authToken: string) {
    return request<ConnectionCredentialOverride>(`/resources/${id}/connection-override`, {}, authToken);
  },
  setConnectionCredentialOverride(id: string, passwordResourceId: string, authToken: string) {
    return request<ConnectionCredentialOverride>(
      `/resources/${id}/connection-override`,
      {
        method: "PUT",
        body: JSON.stringify({ passwordResourceId })
      },
      authToken
    );
  },
  clearConnectionCredentialOverride(id: string, authToken: string) {
    return request<{ status: string }>(
      `/resources/${id}/connection-override`,
      {
        method: "DELETE"
      },
      authToken
    );
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
  createBrowserExtensionConnectToken(authToken: string) {
    return request<BrowserExtensionConnectToken>(
      "/auth/browser-extension-connect-token",
      {
        method: "POST"
      },
      authToken
    );
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
  listAudit(authToken: string) {
    return request<{ items: AuditEvent[] }>("/audit", {}, authToken);
  },
  adminConfig(authToken: string) {
    return request<AdminConfig>("/admin/config", {}, authToken);
  },
  updateAdminConfig(
    input: {
      entraTenantId?: string;
      entraClientId?: string;
      entraAuthority?: string;
      entraRedirectUri?: string;
      entraGroupSource?: string;
      entraEnabled?: boolean;
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
    },
    authToken: string
  ) {
    return request<AdminConfig>(
      "/admin/config",
      {
        method: "PUT",
        body: JSON.stringify(input)
      },
      authToken
    );
  },
  generateRDPSigningTestCertificate(authToken: string) {
    return request<{ enabled: boolean; certificateConfigured: boolean; subject: string; thumbprintSha256: string; generatedAt?: string }>(
      "/admin/rdp-signing/test-certificate",
      {
        method: "POST"
      },
      authToken
    );
  },
  myActivity(authToken: string) {
    return request<{ items: AuditEvent[] }>("/me/activity", {}, authToken);
  },
  myNotifications(authToken: string) {
    return request<{ items: UserNotification[] }>("/me/notifications", {}, authToken);
  },
  markNotificationRead(id: string, authToken: string) {
    return request<{ status: string }>(`/me/notifications/${encodeURIComponent(id)}/read`, { method: "POST" }, authToken);
  },
  listLocalGroups(authToken: string) {
    return request<{ items: LocalGroup[] }>("/admin/local-groups", {}, authToken);
  },
  listArchivedResources(authToken: string) {
    return request<{ items: ArchivedResourceSummary[] }>("/admin/archived-resources", {}, authToken);
  },
  listUsers(authToken: string) {
    return request<{ items: UserSummary[] }>("/admin/users", {}, authToken);
  },
  createAdminUser(input: CreateUserInput, authToken: string) {
    return request<UserAccessDetail>(
      "/admin/users",
      {
        method: "POST",
        body: JSON.stringify(input)
      },
      authToken
    );
  },
  listAdminNotificationDeliveries(authToken: string) {
    return request<{ items: NotificationDeliveryRecord[] }>("/admin/notification-deliveries", {}, authToken);
  },
  getAdminUser(id: string, authToken: string) {
    return request<UserAccessDetail>(`/admin/users/${encodeURIComponent(id)}`, {}, authToken);
  },
  getAdminUserVisibleResources(id: string, authToken: string) {
    return request<{ items: VisibleResourceSummary[] }>(`/admin/users/${encodeURIComponent(id)}/visible-resources`, {}, authToken);
  },
  updateAdminUser(id: string, input: UserAccessUpdateInput, authToken: string) {
    return request<UserAccessDetail>(
      `/admin/users/${encodeURIComponent(id)}`,
      {
        method: "PUT",
        body: JSON.stringify(input)
      },
      authToken
    );
  },
  deleteAdminUser(id: string, authToken: string) {
    return request<{ personalResourcesDeleted: number }>(
      `/admin/users/${encodeURIComponent(id)}`,
      {
        method: "DELETE"
      },
      authToken
    );
  },
  restoreArchivedResource(id: string, authToken: string) {
    return request<{ status: string }>(
      `/admin/archived-resources/${id}/restore`,
      {
        method: "POST"
      },
      authToken
    );
  },
  discoverKeyVault(authToken: string) {
    return request<KeyVaultDiscoverResult>("/keyvault/discover", {}, authToken);
  },
  importKeyVaultSecrets(input: KeyVaultImportForm & { items: KeyVaultImportItem[] }, authToken: string) {
    return request<{ items: Resource[] }>(
      "/keyvault/import",
      {
        method: "POST",
        body: JSON.stringify(input)
      },
      authToken
    );
  },
  syncKeyVault(automatic: boolean, authToken: string) {
    return request<KeyVaultSyncResult>(
      "/keyvault/sync",
      {
        method: "POST",
        body: JSON.stringify({ automatic })
      },
      authToken
    );
  },
  discoverAppRegistrations(authToken: string) {
    return request<AppRegistrationDiscoverResult>("/appregistrations/discover", {}, authToken);
  },
  importAppRegistrations(input: AppRegistrationImportForm & { tenantId: string }, authToken: string) {
    return request<{ items: Resource[] }>(
      "/appregistrations/import",
      {
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
      },
      authToken
    );
  },
  syncAppRegistrations(automatic: boolean, authToken: string) {
    return request<AppRegistrationSyncResult>(
      "/appregistrations/sync",
      {
        method: "POST",
        body: JSON.stringify({ automatic })
      },
      authToken
    );
  },
  updateAppRegistrationNotificationPolicies(
    id: string,
    input: { resourcePolicy?: AppRegistrationNotificationPolicy; credentialPolicies: AppRegistrationCredentialPolicyInput[] },
    authToken: string
  ) {
    return request<Resource>(
      `/resources/${encodeURIComponent(id)}/app-registration-notifications`,
      {
        method: "PUT",
        body: JSON.stringify(input)
      },
      authToken
    );
  },
  createLocalGroup(input: LocalGroupForm, authToken: string) {
    return request<{ status: string }>(
      "/admin/local-groups",
      {
        method: "POST",
        body: JSON.stringify(input)
      },
      authToken
    );
  },
  updateLocalGroup(name: string, input: LocalGroupForm, authToken: string) {
    return request<{ status: string }>(
      `/admin/local-groups/${encodeURIComponent(name)}`,
      {
        method: "PUT",
        body: JSON.stringify(input)
      },
      authToken
    );
  }
};
