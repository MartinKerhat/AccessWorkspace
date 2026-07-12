const DEFAULT_BASE_URL = "http://localhost:8080";
const PENDING_SAVE_KEY = "pendingPortalCredential";
const INSTALLATION_ID_KEY = "installationId";
const CREDENTIAL_SAVE_PROMPTS_ENABLED_KEY = "credentialSavePromptsEnabled";

async function getConfig() {
  const stored = await chrome.storage.local.get([
    "workspaceBaseUrl",
    "sessionToken",
    INSTALLATION_ID_KEY,
    CREDENTIAL_SAVE_PROMPTS_ENABLED_KEY
  ]);
  return {
    workspaceBaseUrl: normalizeBaseUrl(stored.workspaceBaseUrl || DEFAULT_BASE_URL),
    sessionToken: (stored.sessionToken || "").trim(),
    installationId: String(stored[INSTALLATION_ID_KEY] || "").trim(),
    credentialSavePromptsEnabled: stored[CREDENTIAL_SAVE_PROMPTS_ENABLED_KEY] === true
  };
}

function normalizeBaseUrl(value) {
  let next = String(value || "").trim();
  if (!next) {
    next = DEFAULT_BASE_URL;
  }
  next = next.replace(/\/+$/, "");
  if (next.endsWith("/api")) {
    next = next.slice(0, -4);
  }
  return next;
}

async function saveConfig(workspaceBaseUrl, sessionToken) {
  const payload = {
    workspaceBaseUrl: normalizeBaseUrl(workspaceBaseUrl),
    sessionToken: String(sessionToken || "").trim()
  };
  await chrome.storage.local.set(payload);
  return payload;
}

async function setCredentialSavePromptsEnabled(enabled) {
  await chrome.storage.local.set({ [CREDENTIAL_SAVE_PROMPTS_ENABLED_KEY]: enabled === true });
  return { credentialSavePromptsEnabled: enabled === true };
}

async function authState() {
  return requestJSON("/auth/me");
}

async function getInstallationId() {
  const stored = await chrome.storage.local.get([INSTALLATION_ID_KEY]);
  const current = String(stored[INSTALLATION_ID_KEY] || "").trim();
  if (current) {
    return current;
  }
  const next = crypto.randomUUID();
  await chrome.storage.local.set({ [INSTALLATION_ID_KEY]: next });
  return next;
}

async function connectWorkspace(workspaceBaseUrl, connectToken) {
  const installationId = await getInstallationId();
  const normalizedBaseUrl = normalizeBaseUrl(workspaceBaseUrl);
  const response = await fetch(`${normalizedBaseUrl}/api/auth/browser-extension-connect-exchange`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json"
    },
    body: JSON.stringify({
      token: String(connectToken || "").trim(),
      installationId
    })
  });
  const payload = await response.json().catch(() => null);
  if (!response.ok) {
    throw new Error(payload?.error || `Connection failed with status ${response.status}`);
  }
  await saveConfig(normalizedBaseUrl, String(payload?.token || ""));
  return {
    workspaceBaseUrl: normalizedBaseUrl,
    user: payload?.user,
    authMode: payload?.authMode
  };
}

async function disconnectWorkspace() {
  const config = await getConfig();
  if (config.sessionToken) {
    await requestJSON("/auth/logout", { method: "POST" }).catch(() => null);
  }
  await chrome.storage.local.set({
    workspaceBaseUrl: config.workspaceBaseUrl,
    sessionToken: ""
  });
  return { disconnected: true };
}

const SUGGESTION_DISMISS_TTL_MS = 15 * 60 * 1000;

function suggestionDismissKey(tabId, url) {
  try {
    return `suggestionDismissed:${tabId}:${new URL(String(url || "")).host}`;
  } catch {
    return "";
  }
}

function suggestionDismissStore() {
  return chrome.storage.session ?? chrome.storage.local;
}

async function dismissPortalSuggestions(tabId, url) {
  const key = suggestionDismissKey(tabId, url);
  if (typeof tabId !== "number" || !key) {
    return { dismissed: false };
  }
  await suggestionDismissStore().set({ [key]: Date.now() });
  return { dismissed: true };
}

async function isPortalSuggestionDismissed(tabId, url) {
  const key = suggestionDismissKey(tabId, url);
  if (typeof tabId !== "number" || !key) {
    return false;
  }
  const stored = await suggestionDismissStore().get([key]);
  const dismissedAt = Number(stored[key] || 0);
  if (!dismissedAt) {
    return false;
  }
  if (Date.now() - dismissedAt > SUGGESTION_DISMISS_TTL_MS) {
    await suggestionDismissStore().remove(key);
    return false;
  }
  return true;
}

function normalizePortalUrl(rawUrl) {
  try {
    const parsed = new URL(String(rawUrl || "").trim());
    parsed.search = "";
    parsed.hash = "";
    if (parsed.pathname !== "/") {
      parsed.pathname = parsed.pathname.replace(/\/+$/, "") || "/";
    }
    return parsed.toString();
  } catch {
    return String(rawUrl || "").trim();
  }
}

function derivePortalName(candidate) {
  const title = String(candidate?.title || "").trim();
  if (title) {
    return title.slice(0, 120);
  }
  try {
    const parsed = new URL(String(candidate?.url || "").trim());
    return parsed.hostname;
  } catch {
    return "Saved portal login";
  }
}

async function storePendingSaveCandidate(candidate) {
  const config = await getConfig();
  if (!candidate?.username || !candidate?.password || !candidate?.url) {
    return { stored: false };
  }
  const normalizedUrl = normalizePortalUrl(candidate.url);
  let existingMatch = await findPortalCredentialMatch(normalizedUrl, candidate.username).catch(() => null);
  if (existingMatch && !(await ownsPortalCredential(existingMatch))) {
    // Only the owner of a stored credential gets the update prompt; other
    // users signing in with their own credentials must not overwrite it.
    existingMatch = null;
  }
  if (!config.credentialSavePromptsEnabled && !existingMatch) {
    await chrome.storage.local.remove(PENDING_SAVE_KEY);
    return { stored: false, disabled: true };
  }
  const payload = {
    name: derivePortalName(candidate),
    title: String(candidate.title || "").trim(),
    url: normalizedUrl,
    username: String(candidate.username || "").trim(),
    password: String(candidate.password || ""),
    createdAt: Date.now(),
    mode: existingMatch ? "update" : "create",
    existingResourceId: existingMatch?.resourceId || "",
    existingResourceName: existingMatch?.resourceName || ""
  };
  await chrome.storage.local.set({ [PENDING_SAVE_KEY]: payload });
  return { stored: true };
}

async function getPendingSaveCandidate(currentUrl) {
  const config = await getConfig();
  const stored = await chrome.storage.local.get([PENDING_SAVE_KEY]);
  const candidate = stored[PENDING_SAVE_KEY];
  if (!candidate) {
    return null;
  }
  if (!candidate.url || Date.now() - Number(candidate.createdAt || 0) > 10 * 60 * 1000) {
    await chrome.storage.local.remove(PENDING_SAVE_KEY);
    return null;
  }
  const pendingUrl = normalizePortalUrl(candidate.url);
  const currentNormalized = normalizePortalUrl(currentUrl);
  try {
    const pendingParsed = new URL(pendingUrl);
    const currentParsed = new URL(currentNormalized);
    if (pendingParsed.host !== currentParsed.host) {
      return null;
    }
  } catch {
    return null;
  }
  if (!config.credentialSavePromptsEnabled && candidate.mode !== "update") {
    await chrome.storage.local.remove(PENDING_SAVE_KEY);
    return null;
  }
  const state = await authState();
  return {
    ...candidate,
    canSaveShared: Boolean(state.user?.isAdmin || state.capabilities?.canViewAdmin),
    userName: String(state.user?.name || "")
  };
}

async function clearPendingSaveCandidate() {
  await chrome.storage.local.remove(PENDING_SAVE_KEY);
  return { cleared: true };
}

async function listPortalMatches(url) {
  const result = await requestJSON("/browser-extension/portal-match", {
    method: "POST",
    body: { url }
  });
  return Array.isArray(result.items) ? result.items : [];
}

async function findPortalCredentialMatch(url, username) {
  const normalizedUrl = normalizePortalUrl(url);
  const normalizedUsername = String(username || "").trim().toLowerCase();
  if (!normalizedUrl || !normalizedUsername) {
    return null;
  }
  const matches = await listPortalMatches(normalizedUrl);
  return matches.find((item) => String(item.username || "").trim().toLowerCase() === normalizedUsername) || null;
}

async function ownsPortalCredential(match) {
  const ownerUserId = String(match?.ownerUserId || "");
  if (!ownerUserId) {
    return false;
  }
  const state = await authState().catch(() => null);
  const userId = String(state?.user?.id || "");
  return userId !== "" && userId === ownerUserId;
}

function buildPortalCredentialPayload(input, state) {
  return {
    name: String(input.name || "").trim() || derivePortalName(input),
    type: "web_portal",
    personal: Boolean(input.personal),
    description: "Saved from browser extension",
    owner: String(state.user?.name || "").trim(),
    ownerTeam: "",
    environment: "",
    status: "active",
    folderPath: "",
    launchMode: "",
    sourceKind: "manual",
    sourceObjectId: "",
    notes: "",
    targetHost: "",
    targetUrl: normalizePortalUrl(input.url),
    targetSystem: "",
    username: String(input.username || "").trim(),
    connectionDomain: "",
    connectionAdminSession: false,
    connectionAutomaticLogon: false,
    connectionWindowMode: "launcher_default",
    connectionUseMultipleMonitors: false,
    connectionShowConnectionBar: true,
    connectionScreenMode: "launcher_default",
    connectionMacAddress: "",
    vaultName: "",
    objectName: "",
    objectType: "",
    objectVersion: "",
    contentType: "",
    provider: "",
    applicationId: "",
    tenantId: "",
    clientId: "",
    credentialType: "",
    displayNameExternal: "",
    linkedSecretRef: "",
    launchAllowed: false,
    revealAllowed: false,
    copyAllowed: true,
    allowedGroups: [],
    secretMode: "inline",
    secretValue: String(input.password || ""),
    secretReference: ""
  };
}

async function savePortalCredential(input) {
  const state = await authState();
  const payload = buildPortalCredentialPayload(input, state);
  const existingMatch = await findPortalCredentialMatch(payload.targetUrl, payload.username);
  const userId = String(state.user?.id || "");
  const ownsExisting = Boolean(existingMatch?.resourceId) && userId !== "" && String(existingMatch.ownerUserId || "") === userId;
  if (ownsExisting) {
    const existing = await requestJSON(`/resources/${existingMatch.resourceId}`);
    const updated = await requestJSON(`/resources/${existingMatch.resourceId}`, {
      method: "PUT",
      body: {
        ...existing,
        ...payload,
        owner: existing.owner,
        ownerTeam: existing.ownerTeam,
        environment: existing.environment,
        status: existing.status,
        folderPath: existing.folderPath,
        notes: existing.notes,
        launchAllowed: existing.launchAllowed,
        copyAllowed: existing.copyAllowed,
        allowedGroups: existing.allowedGroups,
        personal: existing.personal
      }
    });
    await clearPendingSaveCandidate();
    return {
      id: updated.id,
      name: updated.name,
      personal: updated.personal,
      action: "updated"
    };
  }
  const created = await requestJSON("/resources", {
    method: "POST",
    body: payload
  });
  await clearPendingSaveCandidate();
  return {
    id: created.id,
    name: created.name,
    personal: created.personal,
    action: "created"
  };
}

async function requestJSON(path, options = {}) {
  const config = await getConfig();
  if (!config.sessionToken) {
    throw new Error("Configure the extension with a workspace session token first.");
  }
  const response = await fetch(`${config.workspaceBaseUrl}/api${path}`, {
    method: options.method || "GET",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${config.sessionToken}`
    },
    body: options.body ? JSON.stringify(options.body) : undefined
  });
  const payload = await response.json().catch(() => null);
  if (!response.ok) {
    throw new Error(payload?.error || `Request failed with status ${response.status}`);
  }
  return payload;
}

async function handleMessage(message, sender) {
  switch (message?.type) {
    case "get-config":
      return getConfig();
    case "save-config":
      return saveConfig(message.workspaceBaseUrl, message.sessionToken);
    case "set-credential-save-prompts-enabled":
      return setCredentialSavePromptsEnabled(message.enabled);
    case "check-auth": {
      const config = await getConfig();
      if (!config.sessionToken) {
        return {
          workspaceBaseUrl: config.workspaceBaseUrl,
          connected: false
        };
      }
      const result = await authState();
      return {
        workspaceBaseUrl: config.workspaceBaseUrl,
        user: result.user,
        authMode: result.authMode,
        connected: true
      };
    }
    case "connect-workspace":
      return connectWorkspace(message.workspaceBaseUrl, message.connectToken);
    case "disconnect-workspace":
      return disconnectWorkspace();
    case "portal-matches": {
      if (await isPortalSuggestionDismissed(sender?.tab?.id, message.url)) {
        return [];
      }
      return listPortalMatches(message.url);
    }
    case "dismiss-portal-suggestions":
      return dismissPortalSuggestions(sender?.tab?.id, message.url);
    case "portal-fill":
      return requestJSON("/browser-extension/portal-fill", {
        method: "POST",
        body: {
          resourceId: message.resourceId,
          url: message.url
        }
      });
    case "report-save-candidate":
      return storePendingSaveCandidate(message.candidate);
    case "get-pending-save":
      return getPendingSaveCandidate(message.url);
    case "clear-pending-save":
      return clearPendingSaveCandidate();
    case "save-portal-credential":
      return savePortalCredential(message.input);
    default:
      return null;
  }
}

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  handleMessage(message, sender)
    .then((result) => sendResponse({ ok: true, result }))
    .catch((error) => sendResponse({ ok: false, error: error instanceof Error ? error.message : String(error) }));
  return true;
});
