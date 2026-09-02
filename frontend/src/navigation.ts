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

// Expiry reminder emails link to a single object as `/?resource=<id>`. Only the
// id travels in the link, so the workspace has to look the record up before it
// knows which category hash to move to.
export function requestedResourceId(): string {
  return new URLSearchParams(window.location.search).get("resource")?.trim() ?? "";
}

// Consumed once: the param is dropped from the address bar as soon as it has
// been read, so reloading or bookmarking the page later does not keep pulling
// the selection back to the object that happened to be expiring that day.
export function clearRequestedResourceId() {
  const url = new URL(window.location.href);
  if (!url.searchParams.has("resource")) {
    return;
  }
  url.searchParams.delete("resource");
  window.history.replaceState(null, "", `${url.pathname}${url.search}${url.hash}`);
}
