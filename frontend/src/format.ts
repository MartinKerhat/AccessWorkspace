export function formatShortDate(value?: string) {
  if (!value) {
    return "no expiry";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleDateString();
}

export function formatArtifactSize(bytes: number) {
  if (!bytes || bytes < 0) {
    return "";
  }
  const units = ["B", "KB", "MB", "GB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

export function formatArtifactMeta(artifact: { version?: string; sizeBytes: number; modifiedAt?: string }) {
  const parts: string[] = [];
  if (artifact.version) {
    parts.push(`v${artifact.version}`);
  }
  const size = formatArtifactSize(artifact.sizeBytes);
  if (size) {
    parts.push(size);
  }
  if (artifact.modifiedAt) {
    const date = new Date(artifact.modifiedAt);
    if (!Number.isNaN(date.getTime())) {
      parts.push(`updated ${date.toLocaleDateString()}`);
    }
  }
  return parts.join(" · ");
}
