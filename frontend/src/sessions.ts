import type { SessionInfo } from "./types";

// Shared presentation helpers for session rows (own "Sessions & devices"
// modal and the admin user detail).

export function sessionKindLabel(item: SessionInfo): string {
  return item.kind === "extension" ? "Browser extension" : "Web session";
}

// "Last active 5 min ago" style copy; falls back to the date for anything
// older than a day so the list stays scannable.
export function relativeTime(iso: string, now = Date.now()): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) {
    return "";
  }
  const seconds = Math.max(0, Math.round((now - then) / 1000));
  if (seconds < 60) {
    return "just now";
  }
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) {
    return `${minutes} min ago`;
  }
  const hours = Math.round(minutes / 60);
  if (hours < 24) {
    return `${hours} ${hours === 1 ? "hour" : "hours"} ago`;
  }
  return new Date(iso).toLocaleString();
}
