import type { ResourceSummary, ResourceType } from "./types";

export type WorkspaceCategory = "connections" | "keyvault" | "appregistrations" | "passwords";

export function categoryLabel(category: WorkspaceCategory): string {
  switch (category) {
    case "connections":
      return "Connections";
    case "keyvault":
      return "Key Vault";
    case "appregistrations":
      return "App registrations";
    case "passwords":
      return "Passwords";
    default:
      return category;
  }
}

export function categoryDescription(category: WorkspaceCategory): string {
  switch (category) {
    case "connections":
      return "SSH and RDP access records optimized for operational launch workflows.";
    case "keyvault":
      return "Azure Key Vault-backed objects, starting with secrets and related metadata.";
    case "appregistrations":
      return "Application registrations and related credential-bearing integration records.";
    case "passwords":
      return "Shared passwords and login-style entries for websites, tools, and legacy systems.";
    default:
      return "Operational workspace category.";
  }
}

export function categoryMatchesType(category: WorkspaceCategory, type: ResourceType): boolean {
  switch (category) {
    case "connections":
      return type === "ssh" || type === "rdp";
    case "keyvault":
      return type === "key_vault_secret";
    case "appregistrations":
      return type === "app_registration";
    case "passwords":
      return type === "shared_secret" || type === "web_portal";
    default:
      return false;
  }
}

export function categoryForType(type: ResourceType): WorkspaceCategory {
  if (type === "ssh" || type === "rdp") {
    return "connections";
  }
  if (type === "key_vault_secret") {
    return "keyvault";
  }
  if (type === "app_registration") {
    return "appregistrations";
  }
  return "passwords";
}

export function filterCategoryItems(items: ResourceSummary[], category: WorkspaceCategory): ResourceSummary[] {
  return items.filter((item) => categoryMatchesType(category, item.type));
}

export function availableCategories(items: ResourceSummary[]): WorkspaceCategory[] {
  const ordered: WorkspaceCategory[] = ["connections", "keyvault", "appregistrations", "passwords"];
  return ordered.filter((category) => filterCategoryItems(items, category).length > 0);
}
