import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { Session } from "../types";

// Legacy storage key from the pre-cookie era. Boot migrates any remaining
// token into the httpOnly cookie and removes it; sign-out keeps clearing it
// defensively. Remove once every session predating the cookie migration has
// expired.
export const authTokenStorageKey = "authToken";

export type LoginOptions = {
  localLoginEnabled: boolean;
  microsoftLoginHint: boolean;
};

function loginMessageFromQuery(): string | undefined {
  const params = new URLSearchParams(window.location.search);
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

type UseAuthDeps = {
  setBusy: (busy: boolean) => void;
  setMessage: (message: string | undefined) => void;
};

// Session lifecycle: bootstrap (cookie session + one-time legacy-token
// migration), local sign-in, invite acceptance, password change, and session
// refresh. The token itself lives in the httpOnly cookie — this hook never
// sees it. Sign-out stays in App — it composes the reset of every other hook.
export function useAuth({ setBusy, setMessage }: UseAuthDeps) {
  const [loginOptions, setLoginOptions] = useState<LoginOptions>({
    localLoginEnabled: true,
    microsoftLoginHint: true
  });
  const [session, setSession] = useState<Session | null>(null);
  const [booting, setBooting] = useState(true);
  const [inviteToken, setInviteToken] = useState<string>(() =>
    new URLSearchParams(window.location.search).get("invite") ?? ""
  );

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

  async function bootstrapAuth() {
    setBooting(true);
    try {
      const bootstrap = await api.authBootstrap();
      setLoginOptions({
        localLoginEnabled: bootstrap.localLoginEnabled,
        microsoftLoginHint: bootstrap.microsoftLoginHint
      });

      // One-time migration: a token still in localStorage from before the
      // cookie era becomes the httpOnly cookie, then leaves localStorage.
      const legacyToken = localStorage.getItem(authTokenStorageKey);
      if (legacyToken) {
        try {
          await api.upgradeSessionCookie(legacyToken);
        } catch {
          // Expired or invalid legacy session — the cookie-based authMe below
          // decides whether the user is signed in.
        }
        localStorage.removeItem(authTokenStorageKey);
      }

      const response = await api.authMe();
      setSession({
        user: response.user,
        authMode: response.authMode,
        capabilities: response.capabilities
      });
      if (!window.location.hash) {
        window.location.hash = "#connections";
      }
    } catch (error) {
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
      setSession({
        user: response.user,
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
      setSession({
        user: response.user,
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
      await api.changePassword(currentPassword, newPassword);
      setMessage("Password changed");
      return true;
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Changing password failed");
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function refreshCurrentSession() {
    const response = await api.authMe();
    setSession({
      user: response.user,
      authMode: response.authMode,
      capabilities: response.capabilities
    });
    return response;
  }

  function handleMicrosoftSignIn() {
    window.location.assign(api.microsoftStartUrl());
  }

  return {
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
  };
}
