import { pick, randomBytes, encodeBytes, type ByteEncoding } from "./random";

// Random key material of a given byte length in a given encoding — the
// "openssl rand -base64 32" family: AES keys, HMAC/JWT secrets, Django/Rails
// style SECRET_KEY, Fernet keys (32 bytes base64url).
export function generateKeyBytes(bytes: number, encoding: ByteEncoding): string {
  return encodeBytes(randomBytes(bytes), encoding);
}

export function generateUUID(): string {
  if (typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  // Fallback: RFC 4122 v4 from raw bytes.
  const b = randomBytes(16);
  b[6] = (b[6] & 0x0f) | 0x40;
  b[8] = (b[8] & 0x3f) | 0x80;
  const hex = Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

const BASE62 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";

// API-key style token: optional prefix + N base62 characters (URL- and
// header-safe, no padding, no ambiguity about escaping).
export function generateToken(length: number, prefix = ""): string {
  const count = Math.floor(length);
  if (!Number.isFinite(count) || count < 8 || count > 256) {
    throw new Error("token length must be between 8 and 256 characters");
  }
  const alphabet = Array.from(BASE62);
  let out = "";
  for (let i = 0; i < count; i += 1) {
    out += pick(alphabet);
  }
  const cleanPrefix = prefix.trim();
  return cleanPrefix ? `${cleanPrefix}${out}` : out;
}
