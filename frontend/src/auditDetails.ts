import type { AuditEvent } from "./types";

function text(value: unknown) {
  return typeof value === "string" ? value : "";
}

function shortVersion(value: unknown) {
  const version = text(value);
  return version ? `version ${version.slice(0, 8)}` : "";
}

function formatDate(value: unknown) {
  const raw = text(value);
  if (!raw) {
    return "—";
  }
  const date = new Date(raw);
  return Number.isNaN(date.getTime()) ? raw : date.toLocaleString();
}

function enabledLabel(value: unknown) {
  return value === false ? "disabled" : "enabled";
}

// "label: after (was before)" when the value changed, "label: after" otherwise.
function change(label: string, after: string, before: string | undefined) {
  return before !== undefined && before !== after ? `${label} ${after} (was ${before})` : `${label} ${after}`;
}

// One line of detail for audit events whose metadata says more than the event
// type: which Key Vault version was written, revealed or changed, and for a
// state change what it was before. Empty for everything else.
export function auditEventDetail(item: AuditEvent): string {
  const metadata = item.metadata ?? {};
  switch (item.eventType) {
    case "keyvault_secret_created":
    case "keyvault_secret_value_set": {
      const source = text(metadata.fileName) ? `file ${text(metadata.fileName)}` : text(metadata.inputMode);
      return [text(metadata.vault) && `vault ${text(metadata.vault)}`, shortVersion(metadata.version), source, enabledLabel(metadata.enabled)]
        .filter(Boolean)
        .join(" · ");
    }
    case "keyvault_secret_state_updated": {
      const previous = (metadata.previous ?? undefined) as Record<string, unknown> | undefined;
      return [
        shortVersion(metadata.version),
        previous && previous.enabled !== metadata.enabled
          ? `${enabledLabel(metadata.enabled)} (was ${enabledLabel(previous.enabled)})`
          : enabledLabel(metadata.enabled),
        change("from", formatDate(metadata.notBefore), previous ? formatDate(previous.notBefore) : undefined),
        change("until", formatDate(metadata.expiresAt), previous ? formatDate(previous.expiresAt) : undefined)
      ]
        .filter(Boolean)
        .join(" · ");
    }
    case "resource_revealed":
      return shortVersion(metadata.version);
    default:
      return "";
  }
}
