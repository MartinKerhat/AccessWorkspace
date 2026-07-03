import type { AppRegistrationDiscoverResult, AppRegistrationDiscoveredApp } from "./types";
import { formatShortDate } from "./format";

export function getSelectedAppRegistrationItems(
  selectedApplicationIds: string[],
  items: AppRegistrationDiscoverResult["items"]
): AppRegistrationDiscoveredApp[] {
  const selectedIds = new Set(selectedApplicationIds);
  return items.filter((item) => selectedIds.has(item.id));
}

export function nextDiscoveredCredential(app: AppRegistrationDiscoveredApp) {
  const now = Date.now();
  const future = app.credentials
    .filter((credential) => credential.endDateTime && new Date(credential.endDateTime).getTime() > now)
    .sort((a, b) => new Date(a.endDateTime ?? 0).getTime() - new Date(b.endDateTime ?? 0).getTime());
  if (future[0]) {
    return future[0];
  }
  return app.credentials
    .filter((credential) => credential.endDateTime)
    .sort((a, b) => new Date(b.endDateTime ?? 0).getTime() - new Date(a.endDateTime ?? 0).getTime())[0];
}

export function discoveredCredentialSummary(app: AppRegistrationDiscoveredApp) {
  if (app.credentials.length === 0) {
    return "no secrets or certificates";
  }
  const secretCount = app.credentials.filter((credential) => credential.type === "client_secret").length;
  const certCount = app.credentials.filter((credential) => credential.type === "certificate").length;
  const nextCredential = nextDiscoveredCredential(app);
  const parts = [];
  if (secretCount > 0) {
    parts.push(`${secretCount} secrets`);
  }
  if (certCount > 0) {
    parts.push(`${certCount} certificates`);
  }
  if (nextCredential?.endDateTime) {
    parts.push(`next expiry ${formatShortDate(nextCredential.endDateTime)}`);
  }
  return parts.join(", ");
}
