import type { ResourceType } from "./types";

export function resourceTypeLabel(type: ResourceType): string {
  switch (type) {
    case "rdp":
      return "RDP connection";
    case "ssh":
      return "SSH connection";
    case "web_portal":
      return "Web portal login";
    case "shared_secret":
      return "Saved password";
    case "key_vault_secret":
      return "Key Vault secret";
    case "app_registration":
      return "App registration";
    default:
      return type;
  }
}

export function resourceTypeSummary(type: ResourceType): string {
  switch (type) {
    case "rdp":
      return "Remote desktop access for Windows environments.";
    case "ssh":
      return "Terminal access for operational systems and bastions.";
    case "web_portal":
      return "Portal credential used for browser-based sign-in.";
    case "shared_secret":
      return "Stored username and password for shared or personal reuse.";
    case "key_vault_secret":
      return "Key Vault-backed reference intended for secure retrieval.";
    case "app_registration":
      return "Application or service principal access record.";
    default:
      return "Operational resource.";
  }
}
