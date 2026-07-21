import { useEffect, useState } from "react";
import { api } from "../api/client";
import type {
  BrowserExtensionClient,
  BrowserExtensionConnectState,
  BrowserExtensionConnectToken,
  BrowserExtensionRuntime,
  Session
} from "../types";

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

type UseBrowserExtensionDeps = {
  session: Session | null;
  setBusy: (busy: boolean) => void;
  setMessage: (message: string | undefined) => void;
};

// Browser-extension runtime info (gated on the passwords capability), the
// manager modal, and the postMessage connect handshake with the installed
// extension.
export function useBrowserExtension({ session, setBusy, setMessage }: UseBrowserExtensionDeps) {
  const currentBrowserClient = detectBrowserExtensionClient();
  const [browserExtensionRuntime, setBrowserExtensionRuntime] = useState<BrowserExtensionRuntime | null>(null);
  const [browserExtensionManagerOpen, setBrowserExtensionManagerOpen] = useState(false);
  const [browserExtensionConnectState, setBrowserExtensionConnectState] = useState<BrowserExtensionConnectState | null>(null);

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

  async function handlePrepareBrowserExtensionSession() {
    if (!session) {
      return;
    }
    setMessage(undefined);
    setBusy(true);
    try {
      const connectToken = await api.createBrowserExtensionConnectToken();
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

  const visibleBrowserPackages = browserExtensionRuntime
    ? browserExtensionRuntime.packages.filter(
        (item) => (session?.user.isAdmin ?? false) || item.id !== "extension-firefox-unsigned"
      )
    : [];
  const currentBrowserPackage = browserExtensionRuntime
    ? visibleBrowserPackages.find((item) => item.browser === currentBrowserClient) ?? null
    : null;

  return {
    browserExtensionRuntime,
    browserExtensionManagerOpen,
    setBrowserExtensionManagerOpen,
    browserExtensionConnectState,
    setBrowserExtensionConnectState,
    handlePrepareBrowserExtensionSession,
    visibleBrowserPackages,
    currentBrowserPackage
  };
}
