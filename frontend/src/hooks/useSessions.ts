import { useState } from "react";
import { api } from "../api/client";
import type { Session, SessionInfo } from "../types";

type UseSessionsDeps = {
  session: Session | null;
  setBusy: (busy: boolean) => void;
  setMessage: (message: string | undefined) => void;
  // The user ended the very session they are using: the server already
  // cleared the cookie, so the app drops its signed-in state like a sign-out.
  onCurrentSessionRevoked: () => void;
};

// "Sessions & devices": the caller's live web + extension sessions, with
// revoke-one and sign-out-everywhere-else. The list doubles as the modal's
// open state (non-null = open), mirroring useVault's settings modal.
export function useSessions({ session, setBusy, setMessage, onCurrentSessionRevoked }: UseSessionsDeps) {
  const [sessions, setSessions] = useState<SessionInfo[] | null>(null);

  async function openSessions() {
    if (!session) {
      return;
    }
    setMessage(undefined);
    try {
      const response = await api.sessions();
      setSessions(response.sessions);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Loading sessions failed");
    }
  }

  function closeSessions() {
    setSessions(null);
  }

  async function refreshSessions() {
    try {
      const response = await api.sessions();
      setSessions((current) => (current ? response.sessions : current));
    } catch {
      // Non-fatal: the modal keeps showing the last known list.
    }
  }

  async function revokeSession(item: SessionInfo): Promise<boolean> {
    setBusy(true);
    try {
      const result = await api.revokeSession(item.id);
      if (result.current) {
        setSessions(null);
        onCurrentSessionRevoked();
        return true;
      }
      await refreshSessions();
      setMessage(item.kind === "extension" ? "Browser extension signed out" : "Session signed out");
      return true;
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Signing out the session failed");
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function revokeOtherSessions(): Promise<boolean> {
    setBusy(true);
    try {
      const result = await api.revokeOtherSessions();
      await refreshSessions();
      setMessage(
        result.revoked === 0
          ? "No other sessions to sign out"
          : `Signed out ${result.revoked} other ${result.revoked === 1 ? "session" : "sessions"}`
      );
      return true;
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Signing out other sessions failed");
      return false;
    } finally {
      setBusy(false);
    }
  }

  function reset() {
    setSessions(null);
  }

  return { sessions, openSessions, closeSessions, revokeSession, revokeOtherSessions, reset };
}
