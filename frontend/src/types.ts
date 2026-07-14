export type User = {
  id: string;
  name: string;
  email: string;
  groups: string[];
  localGroups: string[];
  rights: string[];
  directRights?: string[];
  isAdmin: boolean;
};

export type BrowserExtensionClient = "chromium" | "firefox" | "safari" | "unknown";

export type BrowserExtensionConnectState = {
  user: User;
  phase: "connecting" | "connected" | "unavailable";
  error?: string;
};

export type NotificationPolicyModalState =
  | { mode: "closed" }
  | {
      mode: "resource";
      resource: Resource;
      useResourceOverride: boolean;
      draft: AppRegistrationNotificationPolicy;
      credentialDrafts: AppRegistrationCredentialPolicyInput[];
    };

export type NotificationAdminForm = {
  appRegistrationNotificationPolicy: AppRegistrationNotificationPolicy;
  notificationEmailEnabled: boolean;
  notificationEmailHost: string;
  notificationEmailPort: number;
  notificationEmailUsername: string;
  notificationEmailPassword: string;
  notificationEmailFrom: string;
  appRegistrationAutoSyncEnabled: boolean;
  appRegistrationSyncIntervalMinutes: number;
};

export type AdminForm = {
  entraTenantId: string;
  entraClientId: string;
  entraAuthority: string;
  entraRedirectUri: string;
  entraGroupSource: string;
  entraClientSecret: string;
  entraEnabled: boolean;
  azureReaderUseAmbientIdentity: boolean;
  keyVaultSources: KeyVaultSource[];
  rdpSigningEnabled: boolean;
};

export type UserSummary = {
  id: string;
  name: string;
  email: string;
  isAdmin: boolean;
  blocked: boolean;
  localGroups: string[];
  externalGroupCount: number;
  rightsCount: number;
};

export type ResolvedLocalGroup = {
  name: string;
  assignmentSource: string;
  matchedExternalGroup?: string;
  rights: string[];
};

export type UserAccessDetail = {
  id: string;
  name: string;
  email: string;
  isAdmin: boolean;
  blocked: boolean;
  externalGroups: string[];
  resolvedLocalGroups: ResolvedLocalGroup[];
  directAssignedLocalGroups: string[];
  directRights: string[];
  rights: string[];
  capabilities: WorkspaceCapabilities;
};

export type UserAccessUpdateInput = {
  blocked: boolean;
  directLocalGroups: string[];
  directRights: string[];
};

export type CreateUserInput = {
  username: string;
  displayName: string;
  email: string;
  password: string;
  invite: boolean;
  isAdmin: boolean;
  blocked: boolean;
  directLocalGroups: string[];
};

export type VaultPasskeyDescriptor = {
  credentialId: string;
  prfSalt: string;
};

export type VaultStatus = {
  hasVault: boolean;
  unlocked: boolean;
  methods: string[];
  passkeys: VaultPasskeyDescriptor[];
};

export type UserInvite = {
  token: string;
  userId: string;
  purpose: string;
  expiresAt: string;
  personalResourcesDeleted?: number;
  emailSent?: boolean;
};

export type AuthMode = "dev" | "entra";

export type CategoryCapabilities = {
  view: boolean;
  create: boolean;
  import: boolean;
  edit: boolean;
  reveal: boolean;
  launch: boolean;
};

export type WorkspaceCapabilities = {
  categories: Record<string, CategoryCapabilities>;
  canViewActivity: boolean;
  canViewAudit: boolean;
  canViewAdmin: boolean;
};

export type AdminConfig = {
  authMode: string;
  entraTenantId: string;
  entraClientId: string;
  entraAuthority: string;
  entraRedirectUri: string;
  entraGroupSource: string;
  entraClientSecretSet: boolean;
  entraConfigured: boolean;
  entraEnabled: boolean;
  azureReaderUseAmbientIdentity: boolean;
  keyVaultSources: KeyVaultSource[];
  keyVaultSourceCount: number;
  localGroupCount: number;
  directRightsRuleCount: number;
  appRegistrationNotificationPolicy: AppRegistrationNotificationPolicy;
  notificationEmailEnabled: boolean;
  notificationEmailHost: string;
  notificationEmailPort: number;
  notificationEmailUsername: string;
  notificationEmailPasswordSet: boolean;
  notificationEmailFrom: string;
  notificationEmailConfigured: boolean;
  appRegistrationAutoSyncEnabled: boolean;
  appRegistrationSyncIntervalMinutes: number;
  appRegistrationLastSyncedAt?: string;
  appRegistrationLastSyncStatus: string;
  appRegistrationLastSyncError: string;
  appRegistrationLastSyncSummary: string;
  rdpSigning: RDPSigningConfig;
};

export type RDPSigningConfig = {
  enabled: boolean;
  certificateConfigured: boolean;
  subject: string;
  thumbprintSha256: string;
  generatedAt?: string;
};

export type KeyVaultSource = {
  name: string;
  vaultUrl: string;
  syncEnabled: boolean;
  syncIntervalMinutes: number;
  autoImportEnabled: boolean;
  defaultOwner: string;
  defaultOwnerTeam: string;
  defaultEnvironment: string;
  defaultDescription: string;
  defaultNotes: string;
  defaultAllowedGroups: string[];
  lastSyncedAt?: string;
  lastSyncStatus: string;
  lastSyncError: string;
  lastSyncSummary: string;
};

export type KeyVaultDiscoveredSecret = {
  id: string;
  name: string;
  vaultName: string;
  vaultUrl: string;
  contentType: string;
  expiresAt?: string;
  enabled: boolean;
};

export type KeyVaultDiscoverSourceResult = {
  source: KeyVaultSource;
  items: KeyVaultDiscoveredSecret[];
  error?: string;
};

export type KeyVaultDiscoverResult = {
  sources: KeyVaultDiscoverSourceResult[];
};

export type KeyVaultImportForm = {
  owner: string;
  ownerTeam: string;
  environment: string;
  description: string;
  notes: string;
  allowedGroups: string[];
  selectedSecretIds: string[];
};

export type KeyVaultImportItem = {
  vaultUrl: string;
  vaultName: string;
  objectName: string;
  secretId: string;
  contentType: string;
  expiresAt?: string;
  enabled: boolean;
};

export type KeyVaultSyncSource = {
  vaultUrl: string;
  name: string;
  syncEnabled: boolean;
  due: boolean;
  lastSyncedAt?: string;
  lastSyncStatus: string;
  lastSyncError: string;
  lastSyncSummary: string;
  importedCount: number;
  updatedCount: number;
  removedCount: number;
  missingCount: number;
  skippedCount: number;
};

export type KeyVaultSyncResult = {
  attemptedSources: number;
  importedResources: number;
  updatedResources: number;
  removedResources: number;
  missingResources: number;
  skippedResources: number;
  automatic: boolean;
  sources: KeyVaultSyncSource[];
};

export type AppRegistrationCredential = {
  keyId: string;
  credentialType: string;
  displayName: string;
  startDateTime?: string;
  endDateTime?: string;
  hint: string;
  usage: string;
  lastSyncedAt?: string;
  notificationPolicyOverride?: AppRegistrationNotificationPolicy;
};

export type AppRegistrationOwner = {
  ownerId: string;
  ownerType: string;
  displayName: string;
  email: string;
};

export type AppRegistrationDiscoveredCredential = {
  keyId: string;
  displayName: string;
  type: string;
  startDateTime?: string;
  endDateTime?: string;
  hint: string;
  usage: string;
};

export type AppRegistrationDiscoveredOwner = {
  id: string;
  type: string;
  displayName: string;
  email: string;
  userPrincipalName?: string;
};

export type AppRegistrationDiscoveredApp = {
  id: string;
  appId: string;
  displayName: string;
  signInAudience: string;
  publisherDomain: string;
  owners: AppRegistrationDiscoveredOwner[];
  ownerError?: string;
  credentials: AppRegistrationDiscoveredCredential[];
};

export type AppRegistrationDiscoverResult = {
  items: AppRegistrationDiscoveredApp[];
};

export type AppRegistrationImportForm = {
  owner: string;
  ownerTeam: string;
  environment: string;
  description: string;
  notes: string;
  allowedGroups: string[];
  selectedApplicationIds: string[];
};

export type AppRegistrationSyncResult = {
  attemptedResources: number;
  updatedResources: number;
  removedResources: number;
  missingResources: number;
  expiringCredentials: number;
  expiredCredentials: number;
  automatic: boolean;
};

export type LocalGroup = {
  name: string;
  description: string;
  rights: string[];
  mappedExternalGroups: string[];
  assignedUserIds: string[];
};

export type LocalGroupForm = LocalGroup;

export type ResourceType =
  | "rdp"
  | "ssh"
  | "web_portal"
  | "shared_secret"
  | "key_vault_secret"
  | "app_registration";

export type SourceKind = "manual" | "azure_key_vault" | "entra_app_registration";
export type SecretMode = "inline" | "external_reference" | "prompt_on_launch";

export type ResourceSummary = {
  id: string;
  name: string;
  type: ResourceType;
  category: string;
  personal: boolean;
  description: string;
  owner: string;
  ownerUserId: string;
  ownerTeam: string;
  environment: string;
  status: string;
  folderPath: string;
  launchMode: string;
  sourceKind: SourceKind;
  targetHost: string;
  targetPort?: number;
  targetUrl: string;
  targetSystem: string;
  username: string;
  connectionDomain: string;
  vaultName: string;
  objectName: string;
  provider: string;
  applicationId: string;
  credentialExpiresAt?: string;
  expiresAt?: string;
  launchAllowed: boolean;
  revealAllowed: boolean;
  copyAllowed: boolean;
  allowedGroups: string[];
  createdAt: string;
  updatedAt: string;
  archivedAt?: string;
};

export type VisibleResourceSummary = ResourceSummary & {
  categoryAccessRight: string;
  visibilityScope: "administrator" | "owner" | "personal" | "everyone" | "matched_groups";
  matchedLocalGroups: string[];
};

export type Resource = ResourceSummary & {
  sourceObjectId: string;
  lastSyncedAt?: string;
  notes: string;
  objectType: string;
  objectVersion: string;
  contentType: string;
  tenantId: string;
  clientId: string;
  credentialType: string;
  displayNameExternal: string;
  linkedSecretRef: string;
  secret: {
    mode: SecretMode;
    reference: string;
  };
  connectionAdminSession: boolean;
  connectionAutomaticLogon: boolean;
  connectionWindowMode: string;
  connectionUseMultipleMonitors: boolean;
  connectionShowConnectionBar: boolean;
  connectionScreenMode: string;
  connectionMacAddress: string;
  appNotificationPolicyOverride?: AppRegistrationNotificationPolicy;
  appCredentials?: AppRegistrationCredential[];
  appOwners?: AppRegistrationOwner[];
};

export type NotificationChannel = "in_app" | "email";

export type AppRegistrationNotificationPolicy = {
  enabled: boolean;
  reminderDays: number[];
  channels: NotificationChannel[];
};

export type AppRegistrationCredentialPolicyInput = {
  keyId: string;
  policy?: AppRegistrationNotificationPolicy;
};

export type UserNotification = {
  id: string;
  userId: string;
  resourceId: string;
  resourceName: string;
  credentialKeyId: string;
  credentialDisplayName: string;
  credentialType: string;
  credentialEndDateTime?: string;
  reminderDay: number;
  title: string;
  body: string;
  channels: NotificationChannel[];
  readAt?: string;
  emailStatus: string;
  emailSentAt?: string;
  emailError: string;
  createdAt: string;
};

export type NotificationDeliveryRecord = {
  id: string;
  userId: string;
  userName: string;
  userEmail: string;
  resourceId: string;
  resourceName: string;
  credentialKeyId: string;
  credentialDisplayName: string;
  credentialType: string;
  reminderDay: number;
  title: string;
  emailStatus: string;
  emailSentAt?: string;
  emailError: string;
  createdAt: string;
};

export type ArchivedResourceSummary = ResourceSummary & {
  archivedBy: string;
  archivedReason: string;
  archivedEventAt?: string;
};

export type ResourceForm = {
  name: string;
  type: ResourceType;
  personal: boolean;
  description: string;
  owner: string;
  // Honored by the backend only for admins; forced to the creator otherwise.
  ownerUserId: string;
  ownerTeam: string;
  environment: string;
  status: string;
  folderPath: string;
  launchMode: string;
  sourceKind: SourceKind;
  sourceObjectId: string;
  lastSyncedAt?: string;
  notes: string;
  targetHost: string;
  targetPort?: number;
  targetUrl: string;
  targetSystem: string;
  username: string;
  connectionDomain: string;
  connectionAdminSession: boolean;
  connectionAutomaticLogon: boolean;
  connectionWindowMode: string;
  connectionUseMultipleMonitors: boolean;
  connectionShowConnectionBar: boolean;
  connectionScreenMode: string;
  connectionMacAddress: string;
  vaultName: string;
  objectName: string;
  objectType: string;
  objectVersion: string;
  contentType: string;
  expiresAt?: string;
  provider: string;
  applicationId: string;
  tenantId: string;
  clientId: string;
  credentialType: string;
  credentialExpiresAt?: string;
  displayNameExternal: string;
  linkedSecretRef: string;
  launchAllowed: boolean;
  revealAllowed: boolean;
  copyAllowed: boolean;
  allowedGroups: string[];
  secretMode: SecretMode;
  secretValue: string;
  secretReference: string;
};

export type AuditEvent = {
  id: string;
  eventType: string;
  userId: string;
  userName: string;
  resourceId?: string;
  resourceName?: string;
  metadata: Record<string, unknown>;
  createdAt: string;
};

export type RevealResult = {
  resourceId: string;
  secretMode: SecretMode;
  secretValue?: string;
  secretReference?: string;
};

export type LaunchPayload = {
  resourceId: string;
  resourceType: ResourceType;
  method: string;
  target: string;
  command?: string;
  url?: string;
  metadata: Record<string, unknown>;
};

export type DownloadArtifact = {
  name: string;
  category: string;
  version?: string;
  sizeBytes: number;
  modifiedAt?: string;
  downloadUrl: string;
};

export type LauncherRuntime = {
  requiredVersion: string;
  statusUrl: string;
  launchUrl: string;
  downloadUrl: string;
  downloads: DownloadArtifact[];
};

export type BrowserExtensionRuntime = {
  requiredVersion: string;
  browser: string;
  downloadUrl: string;
  packages: BrowserExtensionPackage[];
};

export type BrowserExtensionPackage = {
  id: string;
  browser: string;
  variant?: string;
  label: string;
  status: string;
  packageType: string;
  downloadUrl?: string;
  installUrl?: string;
  actionLabel: string;
  notes: string;
  files: DownloadArtifact[];
};

export type BrowserExtensionConnectToken = {
  token: string;
  user: User;
  authMode: AuthMode;
  expiresAt: string;
};

export type LauncherLocalStatus = {
  version: string;
  ready: boolean;
};

export type ConnectionCredentialOverride = {
  connectionId: string;
  passwordResourceId: string;
  passwordResourceName: string;
  username: string;
  personal: boolean;
  updatedAt?: string;
};
