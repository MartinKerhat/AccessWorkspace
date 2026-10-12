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
  generatorCache = { at: 0, allowed: false };
  return payload;
}

// ---- password / passphrase generation for sign-up forms ----
// Same algorithms and word list as the workspace Generator page. Options
// come from the user's remembered settings (GET /me/generator-preferences,
// fetched fresh for every chip so a change on the web page is picked up at
// once); what the user adjusts on the chip is remembered back the same way
// the web page does it. Generation is offered only to users holding
// generator.passwords.

const DEFAULT_PASSWORD_OPTIONS = {
  length: 20,
  lowercase: true,
  uppercase: true,
  digits: true,
  symbols: true,
  avoidAmbiguous: true,
  safeSymbols: true
};
const DEFAULT_PASSPHRASE_OPTIONS = {
  words: 5,
  separator: "-",
  capitalize: true,
  addDigit: true
};
const CAPABILITY_CACHE_TTL_MS = 60 * 1000;
let generatorCache = { at: 0, allowed: false };

const WORDLIST = [
  "acid", "acorn", "actor", "adapt", "admit", "adult", "agent", "agree", "ahead", "aisle", "alarm",
  "album", "alert", "alien", "alike", "alive", "alley", "allow", "aloud", "amber", "amuse",
  "angle", "angry", "ankle", "apart", "apple", "apron", "arena", "argue", "arise", "armor",
  "arrow", "aside", "asset", "atlas", "attic", "audio", "avoid", "awake", "award", "bacon",
  "badge", "bagel", "baker", "balmy", "banjo", "barge", "basil", "basin", "batch", "beach",
  "beard", "beast", "began", "begin", "belly", "below", "bench", "berry", "birth", "black",
  "blade", "blame", "blank", "blast", "blaze", "bleak", "blend", "bless", "blimp", "blink",
  "block", "bloom", "blunt", "blush", "board", "boast", "bonus", "boost", "booth", "bound",
  "brain", "brake", "brand", "brave", "bread", "break", "breed", "brick", "bride", "brief",
  "bring", "brink", "brisk", "broad", "broke", "brook", "broom", "brush", "buddy", "budge",
  "build", "bulky", "bunch", "burst", "cabin", "cable", "cadet", "camel", "candy", "canoe",
  "cargo", "carry", "carve", "catch", "cause", "cedar", "chair", "chalk", "champ", "chant",
  "chaos", "charm", "chart", "chase", "cheap", "check", "cheek", "cheer", "chess", "chest",
  "chief", "child", "chill", "chime", "choir", "chord", "chunk", "cider", "cigar", "civic",
  "claim", "clamp", "clash", "clasp", "class", "clean", "clear", "clerk", "click", "cliff",
  "climb", "cling", "cloak", "clock", "clone", "close", "cloth", "cloud", "clown", "coach",
  "coast", "cobra", "cocoa", "comet", "comic", "coral", "couch", "cough", "count", "court",
  "cover", "crack", "craft", "crane", "crash", "crate", "crawl", "crazy", "creek", "crest",
  "crisp", "cross", "crowd", "crown", "crumb", "crush", "crust", "cubic", "curly", "curve",
  "cycle", "daily", "dairy", "daisy", "dance", "dandy", "dealt", "debut", "decaf", "decoy",
  "delay", "delta", "dense", "depth", "diary", "digit", "dimly", "diner", "dirty", "disco",
  "ditch", "dizzy", "dodge", "donor", "donut", "doubt", "dough", "draft", "drain", "drama",
  "drape", "dream", "dress", "drift", "drill", "drink", "drive", "droop", "drove", "drowsy",
  "dusty", "dwarf", "eager", "eagle", "early", "earth", "easel", "eight", "elbow", "elder",
  "elect", "elite", "empty", "enjoy", "enter", "entry", "equal", "equip", "erase", "essay",
  "event", "every", "exact", "exert", "exile", "exist", "extra", "fable", "fancy", "favor",
  "feast", "fence", "ferry", "fetch", "fever", "fiber", "field", "fiery", "fifth", "fifty",
  "fight", "final", "first", "flake", "flame", "flash", "fleet", "flesh", "flick", "fling",
  "flint", "float", "flock", "flood", "floor", "flour", "fluid", "flute", "focus", "foggy",
  "force", "forge", "forth", "forum", "found", "frame", "frank", "fresh", "fried", "frost",
  "froze", "fruit", "fudge", "fully", "funny", "gamer", "gauge", "gavel", "giant", "given",
  "glade", "gland", "glare", "glass", "glaze", "gleam", "glide", "globe", "gloom", "glory",
  "glove", "going", "goose", "grace", "grade", "grain", "grand", "grant", "grape", "graph",
  "grasp", "grass", "gravy", "great", "green", "greet", "grief", "grill", "grind", "groan",
  "groom", "group", "grove", "growl", "grown", "guard", "guess", "guest", "guide", "guild",
  "habit", "handy", "happy", "harsh", "haste", "hatch", "haven", "hazel", "heart", "heavy",
  "hedge", "hello", "hence", "herbs", "hinge", "hobby", "honey", "horse", "hotel", "hound",
  "house", "hover", "human", "humid", "humor", "hurry", "husky", "ideal", "image", "index",
  "inner", "input", "irony", "issue", "ivory", "jelly", "jewel", "joint", "jolly", "juice",
  "juicy", "jumbo", "kayak", "kneel", "knife", "knock", "known", "label", "ladle", "lager",
  "lance", "large", "laser", "latch", "later", "laugh", "layer", "leafy", "learn", "lease",
  "ledge", "legal", "lemon", "level", "lever", "light", "lilac", "limit", "linen", "liver",
  "lobby", "local", "lodge", "logic", "loyal", "lucky", "lunar", "lunch", "lyric", "magic",
  "major", "mango", "maple", "march", "marsh", "match", "mayor", "meadow", "medal", "merit",
  "merry", "metal", "meter", "micro", "might", "minor", "minty", "mirth", "model", "moist",
  "money", "month", "moose", "moral", "motor", "mount", "mouse", "mouth", "movie", "mural",
  "music", "naval", "nerve", "never", "newly", "night", "noble", "noise", "north", "novel",
  "nurse", "oasis", "occur", "ocean", "offer", "often", "olive", "onion", "opera", "orbit",
  "order", "organ", "other", "otter", "ounce", "outer", "oxide", "paddle", "paint", "panel",
  "panic", "pants", "paper", "parka", "party", "pasta", "patch", "pause", "peach", "pearl",
  "pedal", "penny", "perch", "phase", "phone", "photo", "piano", "picky", "piece", "pilot",
  "pinch", "pixel", "pizza", "place", "plain", "plane", "plank", "plant", "plate", "plaza",
  "plumb", "plump", "poach", "point", "polar", "porch", "pound", "power", "press", "price",
  "pride", "prime", "print", "prism", "prize", "probe", "prone", "proof", "proud", "prune",
  "pulse", "punch", "pupil", "puppy", "purse", "quake", "queen", "query", "quest", "quick",
  "quiet", "quilt", "quote", "radar", "radio", "rainy", "raise", "rally", "ranch", "range",
  "rapid", "raven", "reach", "ready", "realm", "rebel", "relax", "relay", "reply", "rhyme",
  "ridge", "rifle", "right", "rinse", "ripen", "risky", "rival", "river", "roast", "robin",
  "robot", "rocky", "rough", "round", "route", "rover", "royal", "rugby", "ruler", "rumor",
  "rural", "salad", "salon", "salty", "sandy", "sauce", "scale", "scarf", "scene", "scent",
  "scoop", "scope", "score", "scout", "scrap", "screw", "seize", "sense", "serve", "seven",
  "shade", "shady", "shaft", "shake", "shaky", "shall", "shape", "share", "shark", "sharp",
  "sheep", "sheet", "shelf", "shell", "shift", "shine", "shiny", "shirt", "shock", "shore",
  "short", "shout", "shrub", "sight", "silky", "since", "siren", "sixth", "sixty", "skate",
  "skill", "skirt", "slate", "sleek", "sleep", "slice", "slide", "slope", "small", "smart",
  "smell", "smile", "smoke", "snack", "snake", "sneak", "snore", "snowy", "solar", "solid",
  "solve", "sonic", "sound", "south", "space", "spare", "spark", "speak", "speed", "spell",
  "spend", "spice", "spicy", "spike", "spine", "spoon", "sport", "spray", "squad", "stack",
  "staff", "stage", "stain", "stair", "stamp", "stand", "stark", "start", "steak", "steam",
  "steel", "steep", "steer", "stern", "stick", "stiff", "still", "sting", "stock", "stone",
  "stool", "store", "storm", "story", "stove", "straw", "strip", "study", "stuff", "style",
  "sugar", "suite", "sunny", "super", "surge", "swamp", "swarm", "sweep", "sweet", "swift",
  "swing", "sword", "syrup", "table", "taken", "tally", "tango", "taste", "teach", "tempo",
  "tense", "thank", "theme", "thick", "thing", "think", "third", "thorn", "those", "three",
  "throw", "thumb", "tidal", "tiger", "tight", "timer", "title", "toast", "today", "token",
  "tonic", "topic", "torch", "total", "touch", "tough", "towel", "tower", "trace", "track",
  "trade", "trail", "train", "trait", "trash", "treat", "trend", "trial", "tribe", "trick",
  "troop", "trout", "truck", "truly", "trunk", "trust", "truth", "tulip", "tumble", "tutor",
  "twice", "twist", "ultra", "uncle", "under", "unify", "union", "unite", "until", "upper",
  "upset", "urban", "usual", "vague", "valid", "value", "vapor", "vault", "vegan", "venue",
  "verse", "video", "vigor", "villa", "vinyl", "viola", "visit", "vista", "vital", "vivid",
  "vocal", "voice", "vouch", "wafer", "wagon", "waist", "waltz", "waste", "watch", "water",
  "weary", "weave", "wedge", "weigh", "weird", "whale", "wheat", "wheel", "where", "which",
  "while", "whirl", "whisk", "white", "whole", "widen", "windy", "wiser", "witty", "woken",
  "woman", "woods", "world", "worry", "worth", "would", "wound", "woven", "wrist", "write",
  "wrote", "yacht", "yearn", "yeast", "yield", "young", "youth", "zebra", "zesty"
];

function randomInt(max) {
  const limit = 0x100000000 - (0x100000000 % max);
  const buf = new Uint32Array(1);
  for (;;) {
    crypto.getRandomValues(buf);
    if (buf[0] < limit) {
      return buf[0] % max;
    }
  }
}

function pickOne(items) {
  return items[randomInt(items.length)];
}

function shuffleInPlace(items) {
  for (let i = items.length - 1; i > 0; i -= 1) {
    const j = randomInt(i + 1);
    [items[i], items[j]] = [items[j], items[i]];
  }
  return items;
}

function clampInt(value, min, max, fallback) {
  const n = Math.floor(Number(value));
  if (!Number.isFinite(n)) {
    return fallback;
  }
  return Math.min(max, Math.max(min, n));
}

function generatePassword(options) {
  const length = clampInt(options.length, 8, 64, 20);
  const ambiguous = new Set(["l", "1", "I", "|", "O", "0", "o"]);
  const filter = (chars) => (options.avoidAmbiguous ? Array.from(chars).filter((c) => !ambiguous.has(c)).join("") : chars);
  const classes = [];
  if (options.lowercase) {
    classes.push(filter("abcdefghijklmnopqrstuvwxyz"));
  }
  if (options.uppercase) {
    classes.push(filter("ABCDEFGHIJKLMNOPQRSTUVWXYZ"));
  }
  if (options.digits) {
    classes.push(filter("0123456789"));
  }
  if (options.symbols) {
    classes.push(options.safeSymbols ? "!@#$%*-_=+.?" : "!@#$%^&*()-_=+[]{};:,.<>?/~");
  }
  if (classes.length === 0) {
    classes.push(filter("abcdefghijklmnopqrstuvwxyz"), filter("ABCDEFGHIJKLMNOPQRSTUVWXYZ"), filter("0123456789"));
  }
  const union = Array.from(classes.join(""));
  const chars = classes.map((cls) => pickOne(Array.from(cls)));
  while (chars.length < length) {
    chars.push(pickOne(union));
  }
  return shuffleInPlace(chars).join("");
}

function generatePassphrase(options) {
  const count = clampInt(options.words, 3, 10, 5);
  const words = [];
  for (let i = 0; i < count; i += 1) {
    let word = pickOne(WORDLIST);
    if (options.capitalize) {
      word = word.charAt(0).toUpperCase() + word.slice(1);
    }
    words.push(word);
  }
  if (options.addDigit) {
    const index = randomInt(words.length);
    words[index] = `${words[index]}${randomInt(10)}`;
  }
  return words.join(typeof options.separator === "string" ? options.separator : "-");
}

function mergeOptions(defaults, stored) {
  const out = { ...defaults };
  if (stored && typeof stored === "object") {
    for (const key of Object.keys(defaults)) {
      if (typeof stored[key] === typeof defaults[key]) {
        out[key] = stored[key];
      }
    }
  }
  return out;
}

async function generatorAllowed() {
  const now = Date.now();
  if (now - generatorCache.at < CAPABILITY_CACHE_TTL_MS) {
    return generatorCache.allowed;
  }
  const state = await authState();
  generatorCache = { at: now, allowed: Boolean(state?.capabilities?.generator?.passwords) };
  return generatorCache.allowed;
}

// request: { kind: "password" | "passphrase", overrides: {...} }.
// Returns { allowed:false } when not connected or the user lacks the right,
// otherwise the value plus the effective options so the chip can show them.
async function generateForPage(request) {
  const config = await getConfig();
  if (!config.sessionToken) {
    return { allowed: false };
  }
  const allowed = await generatorAllowed().catch(() => false);
  if (!allowed) {
    return { allowed: false };
  }
  const kind = request?.kind === "passphrase" ? "passphrase" : "password";
  const prefs = await requestJSON("/me/generator-preferences").catch(() => null);
  // Opt-in: the user switches this on in the web app's Browser extensions
  // dialog; without it the extension never offers a generated password.
  if (prefs?.preferences?.extension?.suggestPasswords !== true) {
    return { allowed: false };
  }
  const stored = prefs?.preferences?.[kind];
  const overrides = request?.overrides && typeof request.overrides === "object" ? request.overrides : {};
  if (kind === "passphrase") {
    const options = mergeOptions(DEFAULT_PASSPHRASE_OPTIONS, { ...(stored || {}), ...overrides });
    return { allowed: true, kind, value: generatePassphrase(options), options };
  }
  const options = mergeOptions(DEFAULT_PASSWORD_OPTIONS, { ...(stored || {}), ...overrides });
  return { allowed: true, kind, value: generatePassword(options), options };
}

// The chip's adjustments are remembered like the web page remembers them.
async function rememberGeneratorPreference(part, settings) {
  if (part !== "password" && part !== "passphrase") {
    return { saved: false };
  }
  await requestJSON(`/me/generator-preferences/${part}`, { method: "PUT", body: settings && typeof settings === "object" ? settings : {} });
  return { saved: true };
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
  const generated = Boolean(candidate?.generated);
  // A generated password may be registered before the username exists on
  // the page (the user clicked "Use" first); the save card lets them type it.
  if (!candidate?.password || !candidate?.url || (!candidate?.username && !generated)) {
    return { stored: false };
  }
  const normalizedUrl = normalizePortalUrl(candidate.url);
  let existingMatch = candidate?.username
    ? await findPortalCredentialMatch(normalizedUrl, candidate.username).catch(() => null)
    : null;
  if (existingMatch && !(await ownsPortalCredential(existingMatch))) {
    // Only the owner of a stored credential gets the update prompt; other
    // users signing in with their own credentials must not overwrite it.
    existingMatch = null;
  }
  // The popup's "save prompts" switch governs passwords the extension merely
  // observed. A password the user asked us to generate is never dropped.
  if (!config.credentialSavePromptsEnabled && !existingMatch && !generated) {
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
    existingResourceName: existingMatch?.resourceName || "",
    generated,
    // Registered on "Use" (before submit): keep the card out of the way for
    // a minute so the sign-up form can be completed; the submit capture or
    // "Save now" lifts the deferral, and after a minute it surfaces anyway so
    // a generated password is never silently lost.
    deferredUntil: candidate?.deferred ? Date.now() + 30 * 1000 : 0
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
    if (!sameSite(pendingParsed.hostname, currentParsed.hostname)) {
      return null;
    }
  } catch {
    return null;
  }
  if (candidate.deferredUntil && Date.now() < Number(candidate.deferredUntil)) {
    return null;
  }
  if (!config.credentialSavePromptsEnabled && candidate.mode !== "update" && !candidate.generated) {
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

// Registration pages routinely hand over to another subdomain of the same
// site (registration → login → home), so a pending save follows the site,
// not the exact host. Site = the last two labels of the host name, which is
// right for the common cases and merely a little generous for the rest.
function sameSite(hostA, hostB) {
  const a = String(hostA || "").toLowerCase();
  const b = String(hostB || "").toLowerCase();
  if (a === b) {
    return true;
  }
  const site = (host) => {
    const labels = host.split(".").filter(Boolean);
    return labels.length <= 2 ? host : labels.slice(-2).join(".");
  };
  return site(a) !== "" && site(a) === site(b) && !/^\d+\.\d+\.\d+\.\d+$/.test(a);
}

// Lifts the "Use"-time deferral so the card shows at the next refresh.
async function liftPendingSaveDeferral() {
  const stored = await chrome.storage.local.get([PENDING_SAVE_KEY]);
  const candidate = stored[PENDING_SAVE_KEY];
  if (!candidate?.deferredUntil) {
    return { lifted: false };
  }
  await chrome.storage.local.set({ [PENDING_SAVE_KEY]: { ...candidate, deferredUntil: 0 } });
  return { lifted: true };
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

const NOT_CONNECTED_MESSAGE = "Not connected to a workspace. Open Access Workspace and use Connect extension.";
const SESSION_ENDED_MESSAGE =
  "Your workspace session for this extension has ended (signed out or expired). Open Access Workspace and connect the extension again.";

async function requestJSON(path, options = {}) {
  const config = await getConfig();
  if (!config.sessionToken) {
    throw new Error(NOT_CONNECTED_MESSAGE);
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
  if (response.status === 401) {
    // The session row is gone — revoked from the workspace ("Sessions &
    // devices" / admin), expired, or the user was blocked. The token can
    // never work again, so drop it: the popup then shows the reconnect
    // state instead of a generic failure on every page.
    await chrome.storage.local.set({ workspaceBaseUrl: config.workspaceBaseUrl, sessionToken: "" });
    const error = new Error(SESSION_ENDED_MESSAGE);
    error.status = 401;
    error.sessionEnded = true;
    throw error;
  }
  if (!response.ok) {
    const error = new Error(payload?.error || `Request failed with status ${response.status}`);
    error.status = response.status;
    throw error;
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
      try {
        const result = await authState();
        return {
          workspaceBaseUrl: config.workspaceBaseUrl,
          user: result.user,
          authMode: result.authMode,
          connected: true
        };
      } catch (error) {
        if (error?.sessionEnded) {
          return {
            workspaceBaseUrl: config.workspaceBaseUrl,
            connected: false,
            sessionEnded: true
          };
        }
        throw error;
      }
    }
    case "connect-workspace":
      return connectWorkspace(message.workspaceBaseUrl, message.connectToken);
    case "generate-password":
      return generateForPage({ kind: message.kind, overrides: message.overrides });
    case "remember-generator-preference":
      return rememberGeneratorPreference(message.part, message.settings);
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
    case "lift-pending-save-deferral":
      return liftPendingSaveDeferral();
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
