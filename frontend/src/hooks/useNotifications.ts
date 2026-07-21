import { useState, type Dispatch, type SetStateAction } from "react";
import { api } from "../api/client";
import type {
  NotificationPolicyModalState,
  Resource,
  ResourceSummary,
  Session,
  UserNotification
} from "../types";

type UseNotificationsDeps = {
  session: Session | null;
  setBusy: (busy: boolean) => void;
  setMessage: (message: string | undefined) => void;
  setSelectedResource: Dispatch<SetStateAction<Resource | undefined>>;
  setAllResources: Dispatch<SetStateAction<ResourceSummary[]>>;
};

// User notifications (app-registration reminders) + the per-resource
// notification-policy override modal. The center-popover open state stays in
// App: it shares the outside-click/ESC listener with the account menu.
export function useNotifications({
  session,
  setBusy,
  setMessage,
  setSelectedResource,
  setAllResources
}: UseNotificationsDeps) {
  const [notifications, setNotifications] = useState<UserNotification[]>([]);
  const [notificationPolicyModalState, setNotificationPolicyModalState] = useState<NotificationPolicyModalState>({
    mode: "closed"
  });

  async function loadNotifications() {
    const response = await api.myNotifications();
    setNotifications(response.items);
  }

  async function handleMarkNotificationRead(notificationID: string) {
    if (!session) {
      return;
    }
    try {
      await api.markNotificationRead(notificationID);
      setNotifications((current) =>
        current.map((item) => (item.id === notificationID ? { ...item, readAt: new Date().toISOString() } : item))
      );
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to update notification");
    }
  }

  async function handleSaveNotificationPolicyOverride() {
    if (!session || notificationPolicyModalState.mode !== "resource") {
      return;
    }
    setBusy(true);
    try {
      const resource = await api.updateAppRegistrationNotificationPolicies(
        notificationPolicyModalState.resource.id,
        {
          resourcePolicy: notificationPolicyModalState.useResourceOverride ? notificationPolicyModalState.draft : undefined,
          credentialPolicies: notificationPolicyModalState.credentialDrafts
        }
      );
      setSelectedResource(resource);
      setAllResources((current) => current.map((item) => (item.id === resource.id ? { ...item, status: resource.status } : item)));
      setNotificationPolicyModalState({ mode: "closed" });
      await loadNotifications();
      setMessage("Notification policy updated.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to update notification policy");
    } finally {
      setBusy(false);
    }
  }

  return {
    notifications,
    loadNotifications,
    handleMarkNotificationRead,
    notificationPolicyModalState,
    setNotificationPolicyModalState,
    handleSaveNotificationPolicyOverride
  };
}
