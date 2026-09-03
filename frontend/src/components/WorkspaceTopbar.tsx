import { useEffect, useRef, useState } from "react";
import { pageTitle, type View } from "../navigation";
import type { KeyVaultViewMode } from "../hooks/useKeyVaultAdmin";
import type { User, UserNotification } from "../types";

function notificationUnreadCount(items: UserNotification[]) {
  return items.filter((item) => !item.readAt).length;
}

type WorkspaceTopbarProps = {
  view: View;
  currentUser: User;
  canViewPasswords: boolean;
  showKeyVaultViewToggle: boolean;
  keyVaultViewMode: KeyVaultViewMode;
  onKeyVaultViewModeChange: (mode: KeyVaultViewMode) => void;
  notifications: UserNotification[];
  onMarkNotificationRead: (notificationID: string) => Promise<void>;
  onMarkAllNotificationsRead: () => Promise<void>;
  onOpenNotificationResource: (resourceId: string) => void;
  vaultUnlocked: boolean;
  onOpenVaultSettings: () => void;
  onOpenBrowserExtensions: () => void;
  onOpenChangePassword: () => void;
  onSignOut: () => void;
};

// The workspace header: page title, Key Vault active/archived toggle, and the
// notification-center + account popovers. The popovers own their open state,
// the shared outside-click/ESC listener, and close on view change.
export function WorkspaceTopbar({
  view,
  currentUser,
  canViewPasswords,
  showKeyVaultViewToggle,
  keyVaultViewMode,
  onKeyVaultViewModeChange,
  notifications,
  onMarkNotificationRead,
  onMarkAllNotificationsRead,
  onOpenNotificationResource,
  vaultUnlocked,
  onOpenVaultSettings,
  onOpenBrowserExtensions,
  onOpenChangePassword,
  onSignOut
}: WorkspaceTopbarProps) {
  const [notificationCenterOpen, setNotificationCenterOpen] = useState(false);
  const [accountMenuOpen, setAccountMenuOpen] = useState(false);
  const notificationMenuRef = useRef<HTMLDivElement | null>(null);
  const accountMenuRef = useRef<HTMLDivElement | null>(null);

  // Close the notification and account popovers on any click outside them (or Escape).
  useEffect(() => {
    if (!notificationCenterOpen && !accountMenuOpen) {
      return;
    }
    function handlePointerDown(event: PointerEvent) {
      const target = event.target as Node | null;
      if (!target) {
        return;
      }
      if (notificationCenterOpen && notificationMenuRef.current && !notificationMenuRef.current.contains(target)) {
        setNotificationCenterOpen(false);
      }
      if (accountMenuOpen && accountMenuRef.current && !accountMenuRef.current.contains(target)) {
        setAccountMenuOpen(false);
      }
    }
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setNotificationCenterOpen(false);
        setAccountMenuOpen(false);
      }
    }
    document.addEventListener("pointerdown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("pointerdown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [notificationCenterOpen, accountMenuOpen]);

  useEffect(() => {
    setAccountMenuOpen(false);
  }, [view]);

  return (
    <header className="workspace-topbar">
      <div>
        <p className="eyebrow">Workspace</p>
        <h2>{pageTitle(view)}</h2>
      </div>
      <div className="topbar-actions">
        {showKeyVaultViewToggle ? (
          <div className="segmented-control topbar-segmented-control" role="tablist" aria-label="Key Vault view mode">
            <button
              type="button"
              className={`segmented-button ${keyVaultViewMode === "active" ? "active" : ""}`}
              onClick={() => onKeyVaultViewModeChange("active")}
            >
              Active
            </button>
            <button
              type="button"
              className={`segmented-button ${keyVaultViewMode === "archived" ? "active" : ""}`}
              onClick={() => onKeyVaultViewModeChange("archived")}
            >
              Archived
            </button>
          </div>
        ) : null}
        <div className="account-menu" ref={notificationMenuRef}>
          <button
            className={`session-chip button ghost notification-chip ${notificationUnreadCount(notifications) > 0 ? "has-unread" : ""}`}
            onClick={() => setNotificationCenterOpen((open) => !open)}
          >
            <span className="notification-chip-copy">
              <span className="notification-chip-title">Notifications</span>
              <small className="notification-chip-count">{notificationUnreadCount(notifications)} unread</small>
            </span>
          </button>
          {notificationCenterOpen ? (
            <div className="account-popover notification-popover">
              {/* Outside the scrolling list on purpose: one expiry sweep can fill
                  the list past its own height, and the bulk action has to stay
                  reachable without scrolling back to the top. */}
              <div className="notification-popover-header">
                <p className="eyebrow">Notification center</p>
                {notificationUnreadCount(notifications) > 0 ? (
                  <button
                    type="button"
                    className="notification-mark-all"
                    onClick={() => void onMarkAllNotificationsRead()}
                  >
                    Mark all as read
                  </button>
                ) : null}
              </div>
              {notifications.length === 0 ? (
                <p className="section-copy">No expiry reminders yet.</p>
              ) : (
                <div className="notification-list">
                  {notifications.map((item) => (
                    <div key={item.id} className={`notification-item ${item.readAt ? "read" : "unread"}`}>
                      <button
                        type="button"
                        className="notification-item-open"
                        onClick={() => {
                          onOpenNotificationResource(item.resourceId);
                          setNotificationCenterOpen(false);
                          void onMarkNotificationRead(item.id);
                        }}
                      >
                        <strong>{item.title}</strong>
                        <p>{item.body}</p>
                        <p>{new Date(item.createdAt).toLocaleString()}</p>
                        {item.channels.includes("email") ? (
                          <p>
                            email {item.emailStatus || "pending"}
                            {item.emailError ? `: ${item.emailError}` : ""}
                          </p>
                        ) : null}
                      </button>
                      {!item.readAt ? (
                        <div className="notification-item-actions">
                          <span className="tag">new</span>
                          {/* Deliberately leaves the popover open — dismissing a
                              reminder is not the same intent as opening it, and
                              closing here means reopening and re-scrolling to
                              reach the next one. */}
                          <button
                            type="button"
                            className="notification-item-dismiss"
                            onClick={() => void onMarkNotificationRead(item.id)}
                          >
                            Mark read
                          </button>
                        </div>
                      ) : null}
                    </div>
                  ))}
                </div>
              )}
            </div>
          ) : null}
        </div>
        <div className="account-menu" ref={accountMenuRef}>
          <button className="session-chip button ghost" onClick={() => setAccountMenuOpen((open) => !open)}>
            <span>{currentUser.name}</span>
            <small>{currentUser.isAdmin ? "Admin session" : "Member session"}</small>
          </button>
          {accountMenuOpen ? (
            <div className="account-popover">
              <div className="account-popover-identity">
                <p className="eyebrow">Signed in</p>
                <strong>{currentUser.name}</strong>
                <span>{currentUser.email}</span>
                <span>{currentUser.isAdmin ? "Administrator" : "Standard user"}</span>
              </div>
              <div className="account-popover-menu">
                <button
                  className="menu-item"
                  onClick={() => {
                    setAccountMenuOpen(false);
                    onOpenVaultSettings();
                  }}
                >
                  <span>Personal passwords</span>
                  <small>{vaultUnlocked ? "Unlocked" : "Locked"}</small>
                </button>
                {canViewPasswords ? (
                  <button
                    className="menu-item"
                    onClick={() => {
                      setAccountMenuOpen(false);
                      onOpenBrowserExtensions();
                    }}
                  >
                    <span>Browser extensions</span>
                  </button>
                ) : null}
                <button
                  className="menu-item"
                  onClick={() => {
                    setAccountMenuOpen(false);
                    onOpenChangePassword();
                  }}
                >
                  <span>Change password</span>
                </button>
                <button className="menu-item" onClick={onSignOut}>
                  <span>Sign out</span>
                </button>
              </div>
            </div>
          ) : null}
        </div>
      </div>
    </header>
  );
}
