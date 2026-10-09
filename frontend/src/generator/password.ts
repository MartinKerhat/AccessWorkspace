import { pick, shuffle } from "./random";

export type PasswordOptions = {
  length: number;
  lowercase: boolean;
  uppercase: boolean;
  digits: boolean;
  symbols: boolean;
  // Drop characters that are easy to confuse when read aloud or typed from
  // paper: l 1 I | O 0 o.
  avoidAmbiguous: boolean;
  // Restrict symbols to ones that survive shells, RDP files, URLs and most
  // "special character" validators without escaping.
  safeSymbols: boolean;
};

export const defaultPasswordOptions: PasswordOptions = {
  length: 20,
  lowercase: true,
  uppercase: true,
  digits: true,
  symbols: true,
  avoidAmbiguous: true,
  safeSymbols: true
};

const LOWER = "abcdefghijklmnopqrstuvwxyz";
const UPPER = "ABCDEFGHIJKLMNOPQRSTUVWXYZ";
const DIGITS = "0123456789";
const SYMBOLS_ALL = "!@#$%^&*()-_=+[]{};:,.<>?/~";
const SYMBOLS_SAFE = "!@#$%*-_=+.?";
const AMBIGUOUS = new Set(["l", "1", "I", "|", "O", "0", "o"]);

function classChars(options: PasswordOptions): string[] {
  const classes: string[] = [];
  const filter = (chars: string) =>
    options.avoidAmbiguous ? Array.from(chars).filter((c) => !AMBIGUOUS.has(c)).join("") : chars;
  if (options.lowercase) {
    classes.push(filter(LOWER));
  }
  if (options.uppercase) {
    classes.push(filter(UPPER));
  }
  if (options.digits) {
    classes.push(filter(DIGITS));
  }
  if (options.symbols) {
    classes.push(options.safeSymbols ? SYMBOLS_SAFE : SYMBOLS_ALL);
  }
  return classes;
}

// Generates a password with at least one character from every enabled class
// and the remaining positions drawn uniformly from the union; the result is
// shuffled so the guaranteed characters are not always at the front.
export function generatePassword(options: PasswordOptions = defaultPasswordOptions): string {
  const length = Math.floor(options.length);
  if (!Number.isFinite(length) || length < 4 || length > 256) {
    throw new Error("length must be between 4 and 256");
  }
  const classes = classChars(options);
  if (classes.length === 0) {
    throw new Error("enable at least one character class");
  }
  if (classes.length > length) {
    throw new Error("length is shorter than the number of enabled classes");
  }
  const union = Array.from(classes.join(""));
  const chars: string[] = classes.map((cls) => pick(Array.from(cls)));
  while (chars.length < length) {
    chars.push(pick(union));
  }
  return shuffle(chars).join("");
}

// Rough entropy estimate in bits for display: log2(alphabet) × length.
export function passwordEntropyBits(options: PasswordOptions): number {
  const alphabet = classChars(options).join("").length;
  if (alphabet === 0) {
    return 0;
  }
  return Math.round(Math.log2(alphabet) * Math.floor(options.length));
}
