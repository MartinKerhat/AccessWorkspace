import { randomBytes, toBase64 } from "./random";

// SSH wire-format helpers (RFC 4251) and the OpenSSH private key container
// ("openssh-key-v1", unencrypted) — enough to emit what `ssh-keygen -t
// ed25519` writes, so the result works with OpenSSH, Git hosting deploy
// keys and PuTTY's importer.

const encoder = new TextEncoder();

function uint32(value: number): Uint8Array {
  const out = new Uint8Array(4);
  new DataView(out.buffer).setUint32(0, value >>> 0, false);
  return out;
}

export function sshString(value: Uint8Array | string): Uint8Array {
  const bytes = typeof value === "string" ? encoder.encode(value) : value;
  return concat(uint32(bytes.length), bytes);
}

export function concat(...parts: Uint8Array[]): Uint8Array {
  const total = parts.reduce((sum, part) => sum + part.length, 0);
  const out = new Uint8Array(total);
  let offset = 0;
  for (const part of parts) {
    out.set(part, offset);
    offset += part.length;
  }
  return out;
}

// Public key blob: string "ssh-ed25519" + string (32-byte public key).
export function ed25519PublicBlob(publicKey: Uint8Array): Uint8Array {
  return concat(sshString("ssh-ed25519"), sshString(publicKey));
}

// The `ssh-ed25519 AAAA... comment` line for authorized_keys.
export function ed25519AuthorizedKeyLine(publicKey: Uint8Array, comment: string): string {
  const base = `ssh-ed25519 ${toBase64(ed25519PublicBlob(publicKey))}`;
  const cleanComment = comment.trim();
  return cleanComment ? `${base} ${cleanComment}` : base;
}

// Unencrypted OpenSSH private key file for an Ed25519 key. `seed` is the
// 32-byte private scalar seed, `publicKey` the 32-byte public key.
export function ed25519OpenSSHPrivateKey(seed: Uint8Array, publicKey: Uint8Array, comment: string): string {
  if (seed.length !== 32 || publicKey.length !== 32) {
    throw new Error("ed25519 seed and public key must be 32 bytes each");
  }
  const magic = concat(encoder.encode("openssh-key-v1"), new Uint8Array([0]));
  const publicBlob = ed25519PublicBlob(publicKey);

  // Private section: two identical random check ints, the key, then padding
  // 1,2,3,... up to the cipher block size (8 for "none").
  const check = randomBytes(4);
  let privateSection = concat(
    check,
    check,
    sshString("ssh-ed25519"),
    sshString(publicKey),
    sshString(concat(seed, publicKey)),
    sshString(comment.trim())
  );
  const padding: number[] = [];
  for (let i = 1; privateSection.length % 8 !== 0; i += 1) {
    padding.push(i);
    privateSection = concat(privateSection, new Uint8Array([i]));
  }

  const file = concat(
    magic,
    sshString("none"), // ciphername
    sshString("none"), // kdfname
    sshString(new Uint8Array(0)), // kdfoptions
    uint32(1), // number of keys
    sshString(publicBlob),
    sshString(privateSection)
  );
  const body = toBase64(file).replace(/(.{70})/g, "$1\n").replace(/\n$/, "");
  return `-----BEGIN OPENSSH PRIVATE KEY-----\n${body}\n-----END OPENSSH PRIVATE KEY-----\n`;
}

// Parses the container back far enough to validate structure (used by tests
// and as a self-check after generation): returns the public key found in the
// public blob and in the private section, and the comment.
export function parseEd25519OpenSSHPrivateKey(pemText: string): { publicKey: Uint8Array; privatePublicKey: Uint8Array; comment: string } {
  const body = pemText
    .replace("-----BEGIN OPENSSH PRIVATE KEY-----", "")
    .replace("-----END OPENSSH PRIVATE KEY-----", "")
    .replace(/\s+/g, "");
  const binary = atob(body);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  const view = new DataView(bytes.buffer);
  let offset = 0;
  const readString = () => {
    const length = view.getUint32(offset, false);
    offset += 4;
    const out = bytes.slice(offset, offset + length);
    offset += length;
    return out;
  };
  const decoder = new TextDecoder();
  const magic = decoder.decode(bytes.slice(0, 15));
  if (magic !== "openssh-key-v1\0") {
    throw new Error("not an openssh-key-v1 container");
  }
  offset = 15;
  if (decoder.decode(readString()) !== "none" || decoder.decode(readString()) !== "none") {
    throw new Error("expected an unencrypted key");
  }
  readString(); // kdfoptions
  const keyCount = view.getUint32(offset, false);
  offset += 4;
  if (keyCount !== 1) {
    throw new Error("expected exactly one key");
  }
  const publicBlob = readString();
  const privateSection = readString();

  const pubView = new DataView(publicBlob.buffer, publicBlob.byteOffset, publicBlob.byteLength);
  const typeLength = pubView.getUint32(0, false);
  const keyLength = pubView.getUint32(4 + typeLength, false);
  const publicKey = publicBlob.slice(8 + typeLength, 8 + typeLength + keyLength);

  const privView = new DataView(privateSection.buffer, privateSection.byteOffset, privateSection.byteLength);
  let p = 0;
  const check1 = privView.getUint32(p, false);
  const check2 = privView.getUint32(p + 4, false);
  if (check1 !== check2) {
    throw new Error("check integers differ");
  }
  p = 8;
  const readPrivString = () => {
    const length = privView.getUint32(p, false);
    p += 4;
    const out = privateSection.slice(p, p + length);
    p += length;
    return out;
  };
  readPrivString(); // key type
  const privatePublicKey = readPrivString();
  readPrivString(); // seed || public
  const comment = decoder.decode(readPrivString());
  return { publicKey, privatePublicKey, comment };
}
