import { useEffect, useState } from "react";
import { api } from "../api/client";
import type {
  CreateUserInput,
  LocalGroup,
  LocalGroupForm,
  Session,
  SessionInfo,
  UserAccessDetail,
  UserAccessUpdateInput,
  UserInvite,
  UserSummary,
  VisibleResourceSummary
} from "../types";

type UseAdminUsersDeps = {
  session: Session | null;
  setBusy: (busy: boolean) => void;
  setMessage: (message: string | undefined) => void;
  loadAllResources: () => Promise<unknown>;
  loadAudit: () => Promise<void>;
  refreshCurrentSession: () => Promise<unknown>;
  // Invoked when refreshing the current user's own session fails after an
  // access change: App clears everything it still owns (the hook clears its
  // own state first). Mirrors the old inline forced-reset block exactly.
  onForcedSignOut: (failureMessage: string) => void;
};

// Admin → Users/Groups: known users, local groups, the selected-user detail
// pair (id as source of truth, effect loads the detail), and the user/group
// management handlers.
export function useAdminUsers({
  session,
  setBusy,
  setMessage,
  loadAllResources,
  loadAudit,
  refreshCurrentSession,
  onForcedSignOut
}: UseAdminUsersDeps) {
  const [localGroups, setLocalGroups] = useState<LocalGroup[]>([]);
  const [knownUsers, setKnownUsers] = useState<UserSummary[]>([]);
  const [selectedAdminUserId, setSelectedAdminUserId] = useState<string>();
  const [selectedAdminUser, setSelectedAdminUser] = useState<UserAccessDetail>();
  const [selectedAdminUserResources, setSelectedAdminUserResources] = useState<VisibleResourceSummary[]>([]);
  const [selectedAdminUserSessions, setSelectedAdminUserSessions] = useState<SessionInfo[]>([]);

  useEffect(() => {
    if (!session?.capabilities.canViewAdmin) {
      setSelectedAdminUserId(undefined);
      setSelectedAdminUser(undefined);
      setSelectedAdminUserResources([]);
      setSelectedAdminUserSessions([]);
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
      setSelectedAdminUserSessions([]);
      return;
    }
    void loadAdminUserDetail(selectedAdminUserId);
  }, [selectedAdminUserId, session]);

  async function loadLocalGroups() {
    const response = await api.listLocalGroups();
    setLocalGroups(response.items);
  }

  async function loadKnownUsers() {
    const response = await api.listUsers();
    setKnownUsers(response.items);
  }

  async function loadAdminUserDetail(id: string) {
    const [userResponse, visibleResourcesResponse, sessionsResponse] = await Promise.all([
      api.getAdminUser(id),
      api.getAdminUserVisibleResources(id),
      api.adminUserSessions(id)
    ]);
    setSelectedAdminUser(userResponse);
    setSelectedAdminUserResources(visibleResourcesResponse.items);
    setSelectedAdminUserSessions(sessionsResponse.sessions);
  }

  async function reloadAdminUserSessions(id: string) {
    try {
      const response = await api.adminUserSessions(id);
      setSelectedAdminUserSessions(response.sessions);
    } catch {
      // Non-fatal: the list keeps its last known state.
    }
  }

  // Admin ends one session of the selected user. Revocation is immediate
  // server-side; the user is not blocked and can sign in again.
  async function handleRevokeAdminUserSession(target: UserAccessDetail, item: SessionInfo) {
    if (!session) {
      return;
    }
    setBusy(true);
    try {
      await api.revokeAdminUserSession(target.id, item.id);
      await reloadAdminUserSessions(target.id);
      if (session.capabilities.canViewAudit) {
        await loadAudit();
      }
      setMessage(`Signed out one ${item.kind === "extension" ? "browser extension" : "web"} session of ${target.name}`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Signing out the session failed");
    } finally {
      setBusy(false);
    }
  }

  // "Sign out everywhere" without blocking: every web + extension session and
  // pending connect token of the user is ended.
  async function handleRevokeAllAdminUserSessions(target: UserAccessDetail) {
    if (!session) {
      return;
    }
    const own = target.id === session.user.id;
    const confirmed = window.confirm(
      own
        ? "Sign yourself out everywhere? Every one of your sessions — including this one — will be ended."
        : `Sign out ${target.name} everywhere?\n\nEvery browser and browser extension signed in as them will be signed out immediately. The account stays active and they can sign in again.`
    );
    if (!confirmed) {
      return;
    }
    setBusy(true);
    try {
      const result = await api.revokeAllAdminUserSessions(target.id);
      if (own) {
        // The current session is gone too; the next request lands on 401 and
        // the global handler drops the signed-in UI.
        onForcedSignOut("Signed out everywhere");
        return;
      }
      await reloadAdminUserSessions(target.id);
      if (session.capabilities.canViewAudit) {
        await loadAudit();
      }
      setMessage(
        result.revoked === 0
          ? `${target.name} had no active sessions`
          : `Signed out ${target.name} everywhere (${result.revoked} ${result.revoked === 1 ? "session" : "sessions"})`
      );
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Signing out the user failed");
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
        await api.updateLocalGroup(originalName, input);
        setMessage("Local group updated");
      } else {
        await api.createLocalGroup(input);
        setMessage("Local group created");
      }
      await Promise.all([loadLocalGroups(), loadKnownUsers()]);
      if (selectedAdminUserId) {
        await loadAdminUserDetail(selectedAdminUserId);
      }
      await refreshCurrentSession();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Saving local group failed");
      throw error;
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
      const updated = await api.updateAdminUser(selectedAdminUserId, input);
      setSelectedAdminUser(updated);
      await Promise.all([loadKnownUsers(), loadLocalGroups()]);
      if (session.capabilities.canViewAudit) {
        await loadAudit();
      }
      if (selectedAdminUserId === session.user.id) {
        try {
          await refreshCurrentSession();
        } catch (error) {
          setLocalGroups([]);
          setKnownUsers([]);
          setSelectedAdminUserId(undefined);
          setSelectedAdminUser(undefined);
          setSelectedAdminUserResources([]);
          onForcedSignOut(error instanceof Error ? error.message : "Session refresh failed after updating user access");
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
      const { user: created, invite } = await api.createAdminUser(input);
      setSelectedAdminUserId(created.id);
      setSelectedAdminUser(created);
      await Promise.all([loadKnownUsers(), loadLocalGroups()]);
      await loadAdminUserDetail(created.id);
      if (session.capabilities.canViewAudit) {
        await loadAudit();
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
      const invite = await api.resetUserPassword(target.id);
      await loadKnownUsers();
      await loadAdminUserDetail(target.id);
      if (session.capabilities.canViewAudit) {
        await loadAudit();
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
      const result = await api.deleteAdminUser(target.id);
      if (selectedAdminUserId === target.id) {
        setSelectedAdminUserId(undefined);
        setSelectedAdminUser(undefined);
        setSelectedAdminUserResources([]);
      }
      await Promise.all([
        loadKnownUsers(),
        loadLocalGroups(),
        loadAllResources()
      ]);
      if (session.capabilities.canViewAudit) {
        await loadAudit();
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

  // Sign-out reset: mirrors exactly what App.signOut used to do inline.
  function reset() {
    setLocalGroups([]);
    setKnownUsers([]);
    setSelectedAdminUserId(undefined);
    setSelectedAdminUser(undefined);
    setSelectedAdminUserResources([]);
    setSelectedAdminUserSessions([]);
  }

  return {
    localGroups,
    knownUsers,
    selectedAdminUserId,
    setSelectedAdminUserId,
    selectedAdminUser,
    selectedAdminUserResources,
    selectedAdminUserSessions,
    handleRevokeAdminUserSession,
    handleRevokeAllAdminUserSessions,
    loadLocalGroups,
    loadKnownUsers,
    loadAdminUserDetail,
    handleSaveLocalGroup,
    handleSaveAdminUserAccess,
    handleCreateAdminUser,
    handleResetAdminUserPassword,
    handleDeleteAdminUser,
    reset
  };
}
