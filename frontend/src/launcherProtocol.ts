import type { LaunchPayload } from "./types";

// Hand-off to the desktop launcher through its access-workspace:// handler,
// for browsers that cannot reach the launcher's localhost bridge.
//
// The web app normally talks to the launcher over http://127.0.0.1 (status
// check, then launch). Chromium and Firefox allow that from an HTTPS page;
// WebKit — Safari, and the other browsers built on it — blocks it as mixed
// content, so there the bridge looks exactly like a launcher that is not
// installed. The URL handler is the only way in, and it is one-way: the web
// app cannot check that a launcher exists, which version it is or what it
// supports, and it never learns whether the launch worked. The launcher
// reports its own failures in a dialog, and the workspace still refuses an
// outdated launcher when the ticket is redeemed.

export type BrowserEnvironment = {
  userAgent: string;
  platform: string;
  maxTouchPoints: number;
};

function currentBrowserEnvironment(): BrowserEnvironment {
  if (typeof navigator === "undefined") {
    return { userAgent: "", platform: "", maxTouchPoints: 0 };
  }
  return {
    userAgent: navigator.userAgent || "",
    platform: navigator.platform || "",
    maxTouchPoints: navigator.maxTouchPoints || 0
  };
}

// browserBlocksLauncherBridge reports whether this is a desktop WebKit
// browser. Every Chromium browser also says "AppleWebKit" and "Safari", so
// those are ruled out by their own tokens. iPhone and iPad are left out
// because no launcher exists for them; an iPad asking for the desktop site
// calls itself a Mac and is told apart by its touch screen.
export function browserBlocksLauncherBridge(env: BrowserEnvironment = currentBrowserEnvironment()): boolean {
  const userAgent = env.userAgent.toLowerCase();
  if (!userAgent.includes("applewebkit")) {
    return false;
  }
  if (/chrome|chromium|crios|edg|opr\/|fxios|android/.test(userAgent)) {
    return false;
  }
  if (/iphone|ipad|ipod/.test(userAgent)) {
    return false;
  }
  if (env.platform.toLowerCase().includes("mac") && env.maxTouchPoints > 1) {
    return false;
  }
  return true;
}

// buildLauncherProtocolUri returns the access-workspace:// link for a launch,
// or null when the launch has no ticket. Only what the launcher needs to
// redeem the ticket goes into the link — the connection details and the
// credential stay on the server until the launcher asks for them.
export function buildLauncherProtocolUri(launch: LaunchPayload, resolveUrl: string): string | null {
  const ticket = typeof launch.metadata.launcherTicket === "string" ? launch.metadata.launcherTicket : "";
  if (!ticket || !resolveUrl) {
    return null;
  }
  const payload: LaunchPayload = {
    resourceId: launch.resourceId,
    resourceType: launch.resourceType,
    method: launch.method,
    target: launch.target,
    metadata: {
      launcherTicket: ticket,
      launcherResolveUrl: resolveUrl
    }
  };
  return `access-workspace://launch?payload=${encodeBase64Url(JSON.stringify(payload))}`;
}

function encodeBase64Url(text: string): string {
  let binary = "";
  for (const byte of new TextEncoder().encode(text)) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

// openLauncherProtocol asks the browser to open the link. The browser shows
// its own "allow this page to open the app?" prompt, every time.
export function openLauncherProtocol(uri: string) {
  window.location.href = uri;
}

export const launcherProtocolHandOffMessage =
  "Connection sent to the desktop launcher — allow the browser's prompt to open it. " +
  "This browser cannot check the launcher from the web app, so if nothing opens, install the current launcher from Download launcher.";
