import { categoryLabel, type WorkspaceCategory } from "../workspaceCategories";
import type { View } from "../navigation";
import type { WorkspaceCapabilities } from "../types";

type WorkspaceSidebarProps = {
  view: View;
  visibleCategories: WorkspaceCategory[];
  capabilities: WorkspaceCapabilities;
};

export function WorkspaceSidebar({ view, visibleCategories, capabilities }: WorkspaceSidebarProps) {
  return (
    <aside className="workspace-sidebar">
      <div className="brand-block">
        <p className="eyebrow">Internal access</p>
        <h1>Access Workspace</h1>
        <p className="section-copy">
          Discover shared operational access, use approved actions, and build toward one governed launcher and secret workspace.
        </p>
      </div>

      <nav className="nav-list">
        {visibleCategories.map((category) => (
          <a key={category} className={view === category ? "active" : ""} href={`#${category}`}>
            {categoryLabel(category)}
          </a>
        ))}
        {capabilities.canViewActivity ? (
          <a className={view === "activity" ? "active" : ""} href="#activity">
            Activity
          </a>
        ) : null}
        {capabilities.canViewAudit ? (
          <a className={view === "audit" ? "active" : ""} href="#audit">
            Audit
          </a>
        ) : null}
        {capabilities.canViewAdmin ? (
          <a className={view === "admin" ? "active" : ""} href="#admin">
            Admin
          </a>
        ) : null}
      </nav>

    </aside>
  );
}
