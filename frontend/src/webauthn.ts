// WebAuthn PRF helper for the personal-passwords vault.
//
// The passkey (Windows Hello / Touch ID / security key) is used purely as a
// key source: its PRF extension produces a stable 32-byte secret after a local
// user check. That secret is sent to the backend (over TLS) to wrap/unwrap the
// vault private key — there is no separate password to remember. We do not use
// the passkey as an auth factor to the server (the session token already is),
// so registration challenges are client-generated and assertions are not
// server-verified; the crypto gate is that only the real PRF output unwraps.

const RP_NAME = "Access Workspace";

// WebAuthn option fields want BufferSource; TS's generic Uint8Array/ArrayBuffer
// variance is stricter than the DOM needs, so cast at these boundaries.
function buf(bytes: Uint8Array): BufferSource {
  return bytes as unknown as BufferSource;
}

function randomBytes(length: number): Uint8Array {
  const buffer = new Uint8Array(length);
  crypto.getRandomValues(buffer);
  return buffer;
}

function toBase64(bytes: ArrayBuffer | Uint8Array): string {
  const view = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes);
  let binary = "";
  for (const b of view) {
    binary += String.fromCharCode(b);
  }
  return btoa(binary);
}

function toBase64Url(bytes: ArrayBuffer | Uint8Array): string {
  return toBase64(bytes).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function fromBase64Url(value: string): Uint8Array {
  const padded = value.replace(/-/g, "+").replace(/_/g, "/").padEnd(Math.ceil(value.length / 4) * 4, "=");
  const binary = atob(padded);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

// passkeysSupported reports whether this browser has a usable platform
// authenticator (Windows Hello / Touch ID). PRF support is checked at
// ceremony time — if the extension yields no output, callers fall back to a
// passphrase.
export async function passkeysSupported(): Promise<boolean> {
  if (typeof window === "undefined" || !window.PublicKeyCredential) {
    return false;
  }
  try {
    return await window.PublicKeyCredential.isUserVerifyingPlatformAuthenticatorAvailable();
  } catch {
    return false;
  }
}

export type PasskeyRegistration = {
  credentialId: string;
  prfSalt: string;
  prfSecret: string;
};

class PrfUnavailableError extends Error {
  constructor() {
    super("This device did not return a passkey secret (PRF unsupported). Use a passphrase instead.");
    this.name = "PrfUnavailableError";
  }
}

export const isPrfUnavailable = (error: unknown): boolean => error instanceof PrfUnavailableError;

function prfFirst(credential: PublicKeyCredential): ArrayBuffer | undefined {
  const results = credential.getClientExtensionResults() as {
    prf?: { results?: { first?: ArrayBuffer } };
  };
  return results.prf?.results?.first;
}

// registerPasskey enrolls a new platform credential and evaluates its PRF to
// obtain the wrap secret. Enrollment and PRF-eval are split into create()
// then get() because not every platform returns PRF output during create().
export async function registerPasskey(userId: string, userName: string): Promise<PasskeyRegistration> {
  const prfSalt = randomBytes(32);
  const created = (await navigator.credentials.create({
    publicKey: {
      challenge: buf(randomBytes(32)),
      rp: { id: window.location.hostname, name: RP_NAME },
      user: { id: buf(new TextEncoder().encode(userId)), name: userName, displayName: userName },
      pubKeyCredParams: [
        { type: "public-key", alg: -7 },
        { type: "public-key", alg: -257 }
      ],
      authenticatorSelection: { residentKey: "preferred", userVerification: "required" },
      timeout: 120000,
      extensions: { prf: { eval: { first: buf(prfSalt) } } } as AuthenticationExtensionsClientInputs
    }
  })) as PublicKeyCredential | null;
  if (!created) {
    throw new Error("Passkey setup was cancelled.");
  }

  // Some browsers return the PRF output straight from create(); if not, do a
  // follow-up assertion to evaluate it.
  let prf = prfFirst(created);
  if (!prf) {
    prf = await evaluatePrf(created.rawId, prfSalt);
  }
  if (!prf) {
    throw new PrfUnavailableError();
  }
  return {
    credentialId: toBase64Url(created.rawId),
    prfSalt: toBase64(prfSalt),
    prfSecret: toBase64(prf)
  };
}

async function evaluatePrf(credentialId: ArrayBuffer, prfSalt: Uint8Array): Promise<ArrayBuffer | undefined> {
  const assertion = (await navigator.credentials.get({
    publicKey: {
      challenge: buf(randomBytes(32)),
      allowCredentials: [{ type: "public-key", id: credentialId }],
      userVerification: "required",
      timeout: 120000,
      extensions: { prf: { eval: { first: buf(prfSalt) } } } as AuthenticationExtensionsClientInputs
    }
  })) as PublicKeyCredential | null;
  if (!assertion) {
    return undefined;
  }
  return prfFirst(assertion);
}

export type PasskeyUnlock = {
  credentialId: string;
  prfSecret: string;
};

// unlockWithPasskey runs the Hello ceremony against the registered
// credentials and returns the PRF secret for the one the user verified with.
export async function unlockWithPasskey(
  descriptors: { credentialId: string; prfSalt: string }[]
): Promise<PasskeyUnlock> {
  if (descriptors.length === 0) {
    throw new Error("No passkeys are registered for this account.");
  }
  // All descriptors share the same PRF salt per credential; WebAuthn lets us
  // offer all credentials at once, but PRF eval needs one salt, so we key the
  // eval by the first and match the returned credential to its salt.
  const saltByCredential = new Map(descriptors.map((d) => [d.credentialId, d.prfSalt]));
  const assertion = (await navigator.credentials.get({
    publicKey: {
      challenge: buf(randomBytes(32)),
      allowCredentials: descriptors.map((d) => ({ type: "public-key" as const, id: buf(fromBase64Url(d.credentialId)) })),
      userVerification: "required",
      timeout: 120000,
      extensions: {
        prf: { eval: { first: buf(fromBase64Url(descriptors[0].prfSalt)) } }
      } as AuthenticationExtensionsClientInputs
    }
  })) as PublicKeyCredential | null;
  if (!assertion) {
    throw new Error("Unlock was cancelled.");
  }
  const usedCredentialId = toBase64Url(assertion.rawId);
  // If the authenticator used a different credential than descriptor[0], its
  // salt differs and the PRF output would be wrong; re-evaluate with the
  // correct salt for the credential actually used.
  let prf = prfFirst(assertion);
  const correctSalt = saltByCredential.get(usedCredentialId);
  if (correctSalt && correctSalt !== descriptors[0].prfSalt) {
    prf = await evaluatePrf(assertion.rawId, fromBase64Url(correctSalt));
  }
  if (!prf) {
    throw new PrfUnavailableError();
  }
  return { credentialId: usedCredentialId, prfSecret: toBase64(prf) };
}
