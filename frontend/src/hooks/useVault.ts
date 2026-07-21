import { useEffect, useRef, useState } from "react";
import { api, isVaultLocked } from "../api/client";
import { passkeysSupported, registerPasskey, unlockWithPasskey, isPrfUnavailable } from "../webauthn";
import type { Session, VaultStatus } from "../types";

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
      setVaultPrompt({ status: { hasVault: true, unlocked: false, methods: [], passkeys: [] }, retry });
    }
    return true;
  }

  // Account-menu entry point: when locked, open the setup/unlock prompt; when
  // unlocked, re-lock this session (the meaningful action once unlocked, so
  // the button is never a dead end).
  async function toggleVaultLock() {
    if (!session) {
      return;
    }
    try {
      const status = await api.vaultStatus();
      if (status.unlocked) {
        await api.vaultLock();
        setVaultUnlocked(false);
        setMessage("Personal passwords locked");
        return;
      }
      setVaultPrompt({ status, retry: async () => {} });
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Checking the vault failed");
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
    vaultCheckedTokenRef.current = null;
  }

  return {
    vaultPrompt,
    setVaultPrompt,
    passkeyCapable,
    vaultUnlocked,
    guardVaultLocked,
    toggleVaultLock,
    submitVaultPassphrase,
    submitVaultPasskey,
    reset
  };
}
