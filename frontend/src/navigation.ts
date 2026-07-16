import { categoryLabel, type WorkspaceCategory } from "./workspaceCategories";

export type View = WorkspaceCategory | "activity" | "audit" | "admin";

export function currentView(): View {
  const hash = window.location.hash.replace("#", "");
  if (
    hash === "connections" ||
    hash === "keyvault" ||
    hash === "appregistrations" ||
    hash === "passwords" ||
    hash === "activity" ||
    hash === "audit" ||
    hash === "admin"
  ) {
    return hash;
  }
  return "connections";
}

export function pageTitle(view: View): string {
  switch (view) {
    case "connections":
    case "keyvault":
    case "appregistrations":
    case "passwords":
      return categoryLabel(view);
    case "activity":
      return "Recent activity";
    case "audit":
      return "Audit trail";
    case "admin":
      return "Administration";
    default:
      return "Operational access workspace";
  }
}
