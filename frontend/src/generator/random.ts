// Cryptographic randomness and encodings for the Generator page. Everything
// here runs in the browser; nothing is sent anywhere unless the user chooses
// to store the result somewhere later.

export function randomBytes(length: number): Uint8Array {
  if (!Number.isInteger(length) || length <= 0 || length > 4096) {
    throw new Error("length must be between 1 and 4096 bytes");
  }
  const out = new Uint8Array(length);
  crypto.getRandomValues(out);
  return out;
}

// Uniform integer in [0, max) without modulo bias: draw 32-bit values and
// reject the ones above the largest multiple of max.
export function randomInt(max: number): number {
  if (!Number.isInteger(max) || max <= 0 || max > 0x100000000) {
    throw new Error("max must be between 1 and 2^32");
  }
  const limit = 0x100000000 - (0x100000000 % max);
  const buf = new Uint32Array(1);
  for (;;) {
    crypto.getRandomValues(buf);
    if (buf[0] < limit) {
      return buf[0] % max;
    }
  }
}

export function pick<T>(items: readonly T[]): T {
  if (items.length === 0) {
    throw new Error("cannot pick from an empty list");
  }
  return items[randomInt(items.length)];
}

// Fisher–Yates with unbiased draws.
export function shuffle<T>(items: T[]): T[] {
  for (let i = items.length - 1; i > 0; i -= 1) {
    const j = randomInt(i + 1);
    [items[i], items[j]] = [items[j], items[i]];
  }
  return items;
}

export function toHex(bytes: Uint8Array): string {
  let out = "";
  for (const b of bytes) {
    out += b.toString(16).padStart(2, "0");
  }
  return out;
}

export function toBase64(bytes: Uint8Array): string {
  let binary = "";
  for (const b of bytes) {
    binary += String.fromCharCode(b);
  }
  return btoa(binary);
}

export function toBase64Url(bytes: Uint8Array): string {
  return toBase64(bytes).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export function fromBase64(text: string): Uint8Array {
  const binary = atob(text);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    out[i] = binary.charCodeAt(i);
  }
  return out;
}

export type ByteEncoding = "hex" | "base64" | "base64url";

export function encodeBytes(bytes: Uint8Array, encoding: ByteEncoding): string {
  switch (encoding) {
    case "hex":
      return toHex(bytes);
    case "base64":
      return toBase64(bytes);
    case "base64url":
      return toBase64Url(bytes);
    default:
      throw new Error(`unknown encoding ${String(encoding)}`);
  }
}

// PEM: base64 body wrapped at 64 columns between BEGIN/END markers.
export function toPem(label: string, der: Uint8Array): string {
  const body = toBase64(der).replace(/(.{64})/g, "$1\n").replace(/\n$/, "");
  return `-----BEGIN ${label}-----\n${body}\n-----END ${label}-----\n`;
}
