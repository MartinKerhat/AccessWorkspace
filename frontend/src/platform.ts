// Client-OS detection for platform-specific downloads (the desktop launcher
// ships per-OS builds). Best effort from user-agent data; "" when unknown.

export type LauncherPlatform = "windows" | "linux" | "mac" | "";

export function detectClientLauncherPlatform(): LauncherPlatform {
  if (typeof navigator === "undefined") {
    return "";
  }
  const platform = (navigator.platform || "").toLowerCase();
  const userAgent = (navigator.userAgent || "").toLowerCase();
  if (platform.includes("win") || userAgent.includes("windows")) {
    return "windows";
  }
  if (platform.includes("mac") || userAgent.includes("mac os")) {
    return "mac";
  }
  // Android and ChromeOS also report Linux in places; neither runs the launcher.
  if ((platform.includes("linux") || userAgent.includes("linux")) && !userAgent.includes("android") && !userAgent.includes("cros")) {
    return "linux";
  }
  return "";
}

// matchesLauncherPlatform reports whether a download artifact targets the
// given client platform, based on its backend category ("launcher-windows",
// "launcher-linux", ...) with the file name as fallback.
export function matchesLauncherPlatform(artifact: { category?: string; name: string }, platform: LauncherPlatform): boolean {
  if (!platform) {
    return false;
  }
  if (artifact.category) {
    return artifact.category === `launcher-${platform}`;
  }
  return artifact.name.toLowerCase().includes(platform);
}
