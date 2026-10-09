import { describe, expect, it } from "vitest";
import { encodeBytes, fromBase64, randomBytes, randomInt, toBase64, toBase64Url, toHex, toPem } from "./random";
import { defaultPasswordOptions, generatePassword, passwordEntropyBits } from "./password";
import { defaultPassphraseOptions, generatePassphrase } from "./passphrase";
import { WORDLIST } from "./wordlist";
import { generateKeyBytes, generateToken, generateUUID } from "./tokens";
import { ed25519AuthorizedKeyLine, ed25519OpenSSHPrivateKey, parseEd25519OpenSSHPrivateKey, sshString } from "./ssh";

describe("random + encodings", () => {
  it("produces the requested number of bytes", () => {
    expect(randomBytes(32)).toHaveLength(32);
    expect(() => randomBytes(0)).toThrow();
  });

  it("randomInt stays within range", () => {
    for (let i = 0; i < 1000; i += 1) {
      const n = randomInt(7);
      expect(n).toBeGreaterThanOrEqual(0);
      expect(n).toBeLessThan(7);
    }
  });

  it("encodes bytes in every supported encoding", () => {
    const bytes = new Uint8Array([0, 1, 254, 255, 62, 63]);
    expect(toHex(bytes)).toBe("0001feff3e3f");
    expect(toBase64(bytes)).toBe("AAH+/z4/");
    expect(toBase64Url(bytes)).toBe("AAH-_z4_");
    expect(encodeBytes(bytes, "hex")).toBe(toHex(bytes));
    expect(fromBase64(toBase64(bytes))).toEqual(bytes);
  });

  it("key bytes have the expected encoded length", () => {
    expect(generateKeyBytes(32, "hex")).toHaveLength(64);
    expect(generateKeyBytes(32, "base64")).toHaveLength(44);
    expect(generateKeyBytes(32, "base64url")).toHaveLength(43);
    expect(generateKeyBytes(64, "hex")).toHaveLength(128);
  });

  it("wraps PEM at 64 columns", () => {
    const pem = toPem("TEST", randomBytes(100));
    const lines = pem.trim().split("\n");
    expect(lines[0]).toBe("-----BEGIN TEST-----");
    expect(lines[lines.length - 1]).toBe("-----END TEST-----");
    for (const line of lines.slice(1, -1)) {
      expect(line.length).toBeLessThanOrEqual(64);
    }
  });
});

describe("password", () => {
  it("honours length and includes every enabled class", () => {
    for (let i = 0; i < 50; i += 1) {
      const pw = generatePassword({ ...defaultPasswordOptions, length: 12 });
      expect(pw).toHaveLength(12);
      expect(pw).toMatch(/[a-z]/);
      expect(pw).toMatch(/[A-Z]/);
      expect(pw).toMatch(/[0-9]/);
      expect(pw).toMatch(/[!@#$%*\-_=+.?]/);
      expect(pw).not.toMatch(/[l1IO0o|]/);
    }
  });

  it("can be restricted to a single class", () => {
    const pw = generatePassword({ ...defaultPasswordOptions, length: 10, uppercase: false, digits: false, symbols: false, avoidAmbiguous: false });
    expect(pw).toMatch(/^[a-z]{10}$/);
  });

  it("rejects impossible options", () => {
    expect(() => generatePassword({ ...defaultPasswordOptions, lowercase: false, uppercase: false, digits: false, symbols: false })).toThrow();
    expect(() => generatePassword({ ...defaultPasswordOptions, length: 3 })).toThrow();
  });

  it("reports entropy", () => {
    expect(passwordEntropyBits(defaultPasswordOptions)).toBeGreaterThan(100);
  });
});

describe("passphrase", () => {
  it("joins the requested number of words from the list", () => {
    const phrase = generatePassphrase({ ...defaultPassphraseOptions, words: 5, capitalize: false, addDigit: false });
    const words = phrase.split("-");
    expect(words).toHaveLength(5);
    for (const word of words) {
      expect(WORDLIST).toContain(word);
    }
  });

  it("applies capitalisation and a digit", () => {
    const phrase = generatePassphrase({ words: 4, separator: " ", capitalize: true, addDigit: true });
    const words = phrase.split(" ");
    expect(words).toHaveLength(4);
    expect(words.every((w) => /^[A-Z]/.test(w))).toBe(true);
    expect(words.filter((w) => /\d$/.test(w))).toHaveLength(1);
  });

  it("wordlist is clean", () => {
    const set = new Set(WORDLIST);
    expect(set.size).toBe(WORDLIST.length);
    expect(WORDLIST.length).toBeGreaterThan(500);
    for (const word of WORDLIST) {
      expect(word).toMatch(/^[a-z]{3,7}$/);
    }
  });
});

describe("tokens", () => {
  it("uuid v4 shape", () => {
    expect(generateUUID()).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
  });

  it("prefixed base62 token", () => {
    const token = generateToken(32, "ak_");
    expect(token).toMatch(/^ak_[A-Za-z0-9]{32}$/);
    expect(generateToken(16)).toMatch(/^[A-Za-z0-9]{16}$/);
  });
});

describe("openssh ed25519 container", () => {
  it("round-trips through the parser with matching public keys and comment", () => {
    const seed = randomBytes(32);
    const publicKey = randomBytes(32);
    const pem = ed25519OpenSSHPrivateKey(seed, publicKey, "user@host");
    expect(pem.startsWith("-----BEGIN OPENSSH PRIVATE KEY-----\n")).toBe(true);
    const parsed = parseEd25519OpenSSHPrivateKey(pem);
    expect(parsed.publicKey).toEqual(publicKey);
    expect(parsed.privatePublicKey).toEqual(publicKey);
    expect(parsed.comment).toBe("user@host");
  });

  it("authorized_keys line decodes to the public blob", () => {
    const publicKey = randomBytes(32);
    const line = ed25519AuthorizedKeyLine(publicKey, "deploy");
    const [type, blob, comment] = line.split(" ");
    expect(type).toBe("ssh-ed25519");
    expect(comment).toBe("deploy");
    const bytes = fromBase64(blob);
    // string "ssh-ed25519" (4 + 11) + string key (4 + 32)
    expect(bytes).toHaveLength(51);
    expect(bytes.slice(0, 4)).toEqual(new Uint8Array([0, 0, 0, 11]));
    expect(bytes.slice(15, 19)).toEqual(new Uint8Array([0, 0, 0, 32]));
    expect(bytes.slice(19)).toEqual(publicKey);
  });

  it("sshString length-prefixes", () => {
    expect(sshString("abc")).toEqual(new Uint8Array([0, 0, 0, 3, 97, 98, 99]));
  });
});
