import { toPem } from "./random";
import { ed25519AuthorizedKeyLine, ed25519OpenSSHPrivateKey } from "./ssh";

export type KeypairResult = {
  privateKeyPem: string;
  publicKeyPem: string;
};

async function exportPair(pair: CryptoKeyPair): Promise<KeypairResult> {
  const privateDer = new Uint8Array(await crypto.subtle.exportKey("pkcs8", pair.privateKey));
  const publicDer = new Uint8Array(await crypto.subtle.exportKey("spki", pair.publicKey));
  return {
    privateKeyPem: toPem("PRIVATE KEY", privateDer),
    publicKeyPem: toPem("PUBLIC KEY", publicDer)
  };
}

// RSA keypair as PKCS#8 / SPKI PEM — what `openssl genpkey -algorithm RSA`
// produces. Suitable for JWT RS256 signing keys and general use.
export async function generateRSAKeypair(modulusLength: 2048 | 3072 | 4096): Promise<KeypairResult> {
  const pair = await crypto.subtle.generateKey(
    {
      name: "RSASSA-PKCS1-v1_5",
      modulusLength,
      publicExponent: new Uint8Array([1, 0, 1]),
      hash: "SHA-256"
    },
    true,
    ["sign", "verify"]
  );
  return exportPair(pair);
}

// EC keypair (P-256 / P-384) as PKCS#8 / SPKI PEM — JWT ES256/ES384, modern
// TLS keys.
export async function generateECKeypair(namedCurve: "P-256" | "P-384"): Promise<KeypairResult> {
  const pair = await crypto.subtle.generateKey({ name: "ECDSA", namedCurve }, true, ["sign", "verify"]);
  return exportPair(pair);
}

export type SSHKeypairResult = {
  privateKeyOpenSSH: string;
  publicKeyLine: string;
};

// Whether this browser's WebCrypto can generate Ed25519 keys (Chrome/Edge
// 137+, Firefox 130+, Safari 17+).
export async function ed25519Supported(): Promise<boolean> {
  try {
    await crypto.subtle.generateKey({ name: "Ed25519" } as AlgorithmIdentifier, true, ["sign", "verify"]);
    return true;
  } catch {
    return false;
  }
}

// Ed25519 SSH keypair in the formats `ssh-keygen -t ed25519` writes. The
// PKCS#8 export of an Ed25519 private key is a fixed 48-byte DER structure
// whose last 32 bytes are the seed; the raw public key export is 32 bytes.
export async function generateEd25519SSHKeypair(comment: string): Promise<SSHKeypairResult> {
  const pair = (await crypto.subtle.generateKey({ name: "Ed25519" } as AlgorithmIdentifier, true, [
    "sign",
    "verify"
  ])) as CryptoKeyPair;
  const pkcs8 = new Uint8Array(await crypto.subtle.exportKey("pkcs8", pair.privateKey));
  const publicKey = new Uint8Array(await crypto.subtle.exportKey("raw", pair.publicKey));
  if (pkcs8.length !== 48 || publicKey.length !== 32) {
    throw new Error("unexpected Ed25519 key export sizes");
  }
  const seed = pkcs8.slice(16, 48);
  return {
    privateKeyOpenSSH: ed25519OpenSSHPrivateKey(seed, publicKey, comment),
    publicKeyLine: ed25519AuthorizedKeyLine(publicKey, comment)
  };
}
