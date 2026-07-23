import { useEffect, useRef, useState } from "react";
import { api, isVaultLocked } from "../api/client";
import { passkeysSupported, registerPasskey, unlockWithPasskey, isPrfUnavailable } from "../webauthn";
import type { Session, VaultStatus } from "../types";

// Best-effort default name for a newly enrolled passkey ("Windows PC"); the
// user can edit it before enrolling and rename it later.
export function suggestPasskeyNickname(): string {
  const platform = (navigator.platform || "").toLowerCase();
  if (platform.includes("win")) {
    return "Windows PC";
  }
  if (platform.includes("mac")) {
    return "Mac";
  }
  if (platform.includes("iphone") || platform.includes("ipad")) {
    return "iPhone / iPad";
  }
  if (platform.includes("linux")) {
    return "Linux PC";
  }
  return "This device";
}

type UseVaultDeps = {
  session: Session | null;
  setBusy: (busy: boolean) => void;
  setMessage: (message: string | undefined) => void;
};

// Personal-vault unlock state: the setup/unlock prompt, passkey capability,
// and the 423-retry guard used by every handler that touches personal secrets.
export function useVault({ session, setBusy, setMessage }: UseVaultDeps) {
  const [vaultPrompt, setVaultPrompt] = useState<{ status: VaultStatus; retry: () => Promise<void> } | null>(null);
  const [passkeyCapable, setPasskeyCapable] = useState(false);
  const [vaultUnlocked, setVaultUnlocked] = useState(false);
  // Non-null while the vault settings modal is open; holds the status the
  // management list renders from.
  const [vaultSettings, setVaultSettings] = useState<VaultStatus | null>(null);
  const vaultCheckedTokenRef = useRef<string | null>(null);

  useEffect(() => {
    void passkeysSupported().then(setPasskeyCapable);
  }, []);

  // After sign-in, surface the personal-passwords unlock once per session.
  // Local users arrive already unlocked (their login password derived the
  // key), so status.unlocked is true and nothing shows; SSO users have no
  // such key, so they're prompted to set up (first time) or unlock (Windows
  // Hello / passphrase). Runs once per session token.
  useEffect(() => {
    if (!session) {
      return;
    }
    // Once per signed-in user (the raw token now lives in the httpOnly
    // cookie, invisible to JS); reset() clears this on sign-out so the next
    // login re-checks.
    if (vaultCheckedTokenRef.current === session.user.id) {
      return;
    }
    vaultCheckedTokenRef.current = session.user.id;
    void (async () => {
      try {
        const status = await api.vaultStatus();
        setVaultUnlocked(status.unlocked);
        if (!status.unlocked) {
          setVaultPrompt({ status, retry: async () => {} });
        }
      } catch {
        // Non-fatal: the reactive 423 path still prompts on first personal use.
      }
    })();
  }, [session]);

  // Personal secrets need the vault unlocked in this session. On a 423 the
  // action is parked, the unlock/setup modal opens, and the same action is
  // retried automatically after a successful unlock.
  async function guardVaultLocked(error: unknown, retry: () => Promise<void>): Promise<boolean> {
    if (!isVaultLocked(error) || !session) {
      return false;
    }
    try {
      const status = await api.vaultStatus();
      setVaultPrompt({ status, retry });
    } catch {
      setVaultPrompt({ status: { hasVault: true, unlocked: false, methods: [], passkeys: [], methodDetails: [] }, retry });
    }
    return true;
  }

  // Account-menu entry point: open the vault settings modal (status, lock/
  // unlock, and unlock-method management live there).
  async function openVaultSettings() {
    if (!session) {
      return;
    }
    setMessage(undefined);
    try {
      const status = await api.vaultStatus();
      setVaultUnlocked(status.unlocked);
      setVaultSettings(status);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Checking the vault failed");
    }
  }

  function closeVaultSettings() {
    setVaultSettings(null);
  }

  // Silent re-fetch after any mutation while the settings modal is open.
  async function refreshVaultSettings() {
    try {
      const status = await api.vaultStatus();
      setVaultUnlocked(status.unlocked);
      setVaultSettings((current) => (current ? status : current));
    } catch {
      // Non-fatal: the modal keeps showing the last known state.
    }
  }

  // Settings-modal lock action.
  async function lockVault() {
    try {
      await api.vaultLock();
      setVaultUnlocked(false);
      await refreshVaultSettings();
      setMessage("Personal passwords locked");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Locking the vault failed");
    }
  }

  // Settings-modal unlock action: routes through the existing setup/unlock
  // prompt (passkey/passphrase choice included) and refreshes the list after.
  async function unlockFromSettings() {
    if (!session) {
      return;
    }
    try {
      const status = await api.vaultStatus();
      setVaultPrompt({ status, retry: refreshVaultSettings });
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Checking the vault failed");
    }
  }

  async function addVaultPassphrase(passphrase: string): Promise<boolean> {
    setBusy(true);
    try {
      await api.vaultAddPassphrase(passphrase);
      await refreshVaultSettings();
      setMessage("Passphrase saved");
      return true;
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Saving the passphrase failed");
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function addVaultPasskey(nickname: string): Promise<boolean> {
    if (!session) {
      return false;
    }
    setBusy(true);
    try {
      const registration = await registerPasskey(session.user.id, session.user.name);
      await api.vaultPasskeyAdd({ ...registration, nickname });
      await refreshVaultSettings();
      setMessage("Passkey added for this device");
      return true;
    } catch (error) {
      if (isPrfUnavailable(error)) {
        setMessage("This device can't use a passkey for personal passwords. Use a passphrase instead.");
      } else {
        setMessage(error instanceof Error ? error.message : "Adding the passkey failed");
      }
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function removeVaultMethod(method: string, label: string): Promise<boolean> {
    setBusy(true);
    try {
      await api.vaultMethodRemove(method, label);
      await refreshVaultSettings();
      setMessage("Unlock method removed");
      return true;
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Removing the unlock method failed");
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function renameVaultPasskey(credentialId: string, nickname: string): Promise<boolean> {
    setBusy(true);
    try {
      await api.vaultMethodRename(credentialId, nickname);
      await refreshVaultSettings();
      return true;
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Renaming the passkey failed");
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function afterVaultUnlocked(): Promise<boolean> {
    const retry = vaultPrompt?.retry;
    setVaultPrompt(null);
    setVaultUnlocked(true);
    if (retry) {
      await retry();
    }
    return true;
  }

  async function submitVaultPassphrase(passphrase: string): Promise<boolean> {
    if (!session || !vaultPrompt) {
      return false;
    }
    setBusy(true);
    try {
      if (vaultPrompt.status.hasVault) {
        await api.vaultUnlock(passphrase);
      } else {
        await api.vaultSetup(passphrase);
      }
      return await afterVaultUnlocked();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unlocking the vault failed");
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function submitVaultPasskey(): Promise<boolean> {
    if (!session || !vaultPrompt) {
      return false;
    }
    setBusy(true);
    try {
      if (vaultPrompt.status.hasVault) {
        const unlock = await unlockWithPasskey(vaultPrompt.status.passkeys);
        await api.vaultPasskeyUnlock(unlock);
      } else {
        const registration = await registerPasskey(session.user.id, session.user.name);
        await api.vaultPasskeySetup(registration);
      }
      return await afterVaultUnlocked();
    } catch (error) {
      if (isPrfUnavailable(error)) {
        setMessage("This device can't use a passkey for personal passwords. Use a passphrase instead.");
      } else {
        setMessage(error instanceof Error ? error.message : "Passkey unlock failed");
      }
      return false;
    } finally {
      setBusy(false);
    }
  }

  // Sign-out reset: mirrors exactly what App.signOut used to do inline.
  function reset() {
    setVaultUnlocked(false);
    setVaultPrompt(null);
    setVaultSettings(null);
    vaultCheckedTokenRef.current = null;
  }

  return {
    vaultPrompt,
    setVaultPrompt,
    passkeyCapable,
    vaultUnlocked,
    vaultSettings,
    guardVaultLocked,
    openVaultSettings,
    closeVaultSettings,
    lockVault,
    unlockFromSettings,
    addVaultPassphrase,
    addVaultPasskey,
    removeVaultMethod,
    renameVaultPasskey,
    submitVaultPassphrase,
    submitVaultPasskey,
    reset
  };
}
