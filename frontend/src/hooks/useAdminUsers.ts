import { useEffect, useState } from "react";
import { api } from "../api/client";
import type {
  CreateUserInput,
  LocalGroup,
  LocalGroupForm,
  Session,
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
  loadAllResources: (authToken: string) => Promise<unknown>;
  loadAudit: (authToken: string) => Promise<void>;
  refreshCurrentSession: (authToken: string) => Promise<unknown>;
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

  useEffect(() => {
    if (!session?.capabilities.canViewAdmin) {
      setSelectedAdminUserId(undefined);
      setSelectedAdminUser(undefined);
      setSelectedAdminUserResources([]);
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
      return;
    }
    void loadAdminUserDetail(selectedAdminUserId, session.authToken);
  }, [selectedAdminUserId, session]);

  async function loadLocalGroups(authToken: string) {
    const response = await api.listLocalGroups(authToken);
    setLocalGroups(response.items);
  }

  async function loadKnownUsers(authToken: string) {
    const response = await api.listUsers(authToken);
    setKnownUsers(response.items);
  }

  async function loadAdminUserDetail(id: string, authToken: string) {
    const [userResponse, visibleResourcesResponse] = await Promise.all([
      api.getAdminUser(id, authToken),
      api.getAdminUserVisibleResources(id, authToken)
    ]);
    setSelectedAdminUser(userResponse);
    setSelectedAdminUserResources(visibleResourcesResponse.items);
  }

  async function handleSaveLocalGroup(mode: "create" | "edit", originalName: string | undefined, input: LocalGroupForm) {
    if (!session) {
      return;
    }
    setBusy(true);
    try {
      if (mode === "edit" && originalName) {
        await api.updateLocalGroup(originalName, input, session.authToken);
        setMessage("Local group updated");
      } else {
        await api.createLocalGroup(input, session.authToken);
        setMessage("Local group created");
      }
      await Promise.all([loadLocalGroups(session.authToken), loadKnownUsers(session.authToken)]);
      if (selectedAdminUserId) {
        await loadAdminUserDetail(selectedAdminUserId, session.authToken);
      }
      await refreshCurrentSession(session.authToken);
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
      const updated = await api.updateAdminUser(selectedAdminUserId, input, session.authToken);
      setSelectedAdminUser(updated);
      await Promise.all([loadKnownUsers(session.authToken), loadLocalGroups(session.authToken)]);
      if (session.capabilities.canViewAudit) {
        await loadAudit(session.authToken);
      }
      if (selectedAdminUserId === session.user.id) {
        try {
          await refreshCurrentSession(session.authToken);
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
      const { user: created, invite } = await api.createAdminUser(input, session.authToken);
      setSelectedAdminUserId(created.id);
      setSelectedAdminUser(created);
      await Promise.all([loadKnownUsers(session.authToken), loadLocalGroups(session.authToken)]);
      await loadAdminUserDetail(created.id, session.authToken);
      if (session.capabilities.canViewAudit) {
        await loadAudit(session.authToken);
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
      const invite = await api.resetUserPassword(target.id, session.authToken);
      await loadKnownUsers(session.authToken);
      await loadAdminUserDetail(target.id, session.authToken);
      if (session.capabilities.canViewAudit) {
        await loadAudit(session.authToken);
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
      const result = await api.deleteAdminUser(target.id, session.authToken);
      if (selectedAdminUserId === target.id) {
        setSelectedAdminUserId(undefined);
        setSelectedAdminUser(undefined);
        setSelectedAdminUserResources([]);
      }
      await Promise.all([
        loadKnownUsers(session.authToken),
        loadLocalGroups(session.authToken),
        loadAllResources(session.authToken)
      ]);
      if (session.capabilities.canViewAudit) {
        await loadAudit(session.authToken);
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
  }

  return {
    localGroups,
    knownUsers,
    selectedAdminUserId,
    setSelectedAdminUserId,
    selectedAdminUser,
    selectedAdminUserResources,
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
