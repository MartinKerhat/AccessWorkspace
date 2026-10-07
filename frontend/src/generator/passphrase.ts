import { pick, randomInt } from "./random";
import { WORDLIST } from "./wordlist";

export type PassphraseOptions = {
  words: number;
  separator: string;
  // Capitalise the first letter of every word (adds a class for validators).
  capitalize: boolean;
  // Append one random digit to a random word.
  addDigit: boolean;
};

export const defaultPassphraseOptions: PassphraseOptions = {
  words: 5,
  separator: "-",
  capitalize: true,
  addDigit: true
};

export function generatePassphrase(options: PassphraseOptions = defaultPassphraseOptions): string {
  const count = Math.floor(options.words);
  if (!Number.isFinite(count) || count < 3 || count > 12) {
    throw new Error("word count must be between 3 and 12");
  }
  const words: string[] = [];
  for (let i = 0; i < count; i += 1) {
    let word = pick(WORDLIST);
    if (options.capitalize) {
      word = word.charAt(0).toUpperCase() + word.slice(1);
    }
    words.push(word);
  }
  if (options.addDigit) {
    const index = randomInt(words.length);
    words[index] = `${words[index]}${randomInt(10)}`;
  }
  return words.join(options.separator);
}

export function passphraseEntropyBits(options: PassphraseOptions): number {
  const perWord = Math.log2(WORDLIST.length);
  let bits = perWord * Math.floor(options.words);
  if (options.addDigit) {
    bits += Math.log2(10 * Math.floor(options.words));
  }
  return Math.round(bits);
}
