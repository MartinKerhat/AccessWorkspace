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
  onOpenNotificationResource: (resourceId: string) => void;
  vaultUnlocked: boolean;
  onToggleVaultLock: () => Promise<void>;
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
  onOpenNotificationResource,
  vaultUnlocked,
  onToggleVaultLock,
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
              <p className="eyebrow">Notification center</p>
              {notifications.length === 0 ? (
                <p className="section-copy">No app registration reminders yet.</p>
              ) : (
                <div className="notification-list">
                  {notifications.map((item) => (
                    <button
                      key={item.id}
                      type="button"
                      className={`notification-item ${item.readAt ? "read" : "unread"}`}
                      onClick={() => {
                        onOpenNotificationResource(item.resourceId);
                        setNotificationCenterOpen(false);
                        void onMarkNotificationRead(item.id);
                      }}
                    >
                      <div>
                        <strong>{item.title}</strong>
                        <p>{item.body}</p>
                        <p>{new Date(item.createdAt).toLocaleString()}</p>
                        {item.channels.includes("email") ? (
                          <p>
                            email {item.emailStatus || "pending"}
                            {item.emailError ? `: ${item.emailError}` : ""}
                          </p>
                        ) : null}
                      </div>
                      {!item.readAt ? <span className="tag">new</span> : null}
                    </button>
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
              <p className="eyebrow">Signed in</p>
              <strong>{currentUser.name}</strong>
              <span>{currentUser.email}</span>
              <span>{currentUser.isAdmin ? "Administrator" : "Standard user"}</span>
              {canViewPasswords ? (
                <button
                  className="button ghost"
                  onClick={() => {
                    setAccountMenuOpen(false);
                    onOpenBrowserExtensions();
                  }}
                >
                  Browser extensions
                </button>
              ) : null}
              <button
                className="button ghost"
                onClick={() => {
                  setAccountMenuOpen(false);
                  void onToggleVaultLock();
                }}
              >
                {vaultUnlocked ? "Lock personal passwords" : "Unlock personal passwords"}
              </button>
              <button
                className="button ghost"
                onClick={() => {
                  setAccountMenuOpen(false);
                  onOpenChangePassword();
                }}
              >
                Change password
              </button>
              <button className="button ghost" onClick={onSignOut}>
                Sign out
              </button>
            </div>
          ) : null}
        </div>
      </div>
    </header>
  );
}
