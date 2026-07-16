import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { LauncherRuntime, Resource } from "../types";

type UseLauncherDeps = {
  selectedResource: Resource | undefined;
};

// Desktop-launcher runtime info + the downloads modal. The runtime is loaded
// lazily when an rdp/ssh resource is selected (it is public metadata, no auth).
export function useLauncher({ selectedResource }: UseLauncherDeps) {
  const [launcherRuntime, setLauncherRuntime] = useState<LauncherRuntime | null>(null);
  const [launcherDownloadsOpen, setLauncherDownloadsOpen] = useState(false);

  useEffect(() => {
    let cancelled = false;

    async function loadLauncherRuntimeForSelection() {
      if (!selectedResource || (selectedResource.type !== "rdp" && selectedResource.type !== "ssh")) {
        return;
      }
      try {
        const runtime = await api.launcherRuntime();
        if (cancelled) {
          return;
        }
        setLauncherRuntime(runtime);
      } catch {
        if (!cancelled) {
          setLauncherRuntime(null);
        }
      }
    }

    void loadLauncherRuntimeForSelection();
    return () => {
      cancelled = true;
    };
  }, [selectedResource]);

  async function refreshLauncherStatus(runtimeArg?: LauncherRuntime | null) {
    const runtime = runtimeArg ?? launcherRuntime;
    if (!runtime) {
      return null;
    }
    try {
      return await api.launcherLocalStatus(runtime.statusUrl);
    } catch {
      return null;
    }
  }

  return {
    launcherRuntime,
    setLauncherRuntime,
    launcherDownloadsOpen,
    setLauncherDownloadsOpen,
    refreshLauncherStatus
  };
}
