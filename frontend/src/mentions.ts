import type { MentionCandidate, MentionTarget } from "./types";

// Storage format for an object mention inside the plain-text notes column:
//   @[Display name](passwords:<id>)
// Markdown-link-shaped so it degrades legibly if it is ever shown unparsed, and
// parseable with one anchored pattern. Anything that does not match — a stray
// "@" in prose, a hand-mangled token — stays literal text and must never throw.
//
// The id is deliberately NOT constrained to a uuid shape: resource ids are
// arbitrary strings (seeded objects use slugs like "res-web"), and requiring a
// uuid silently left those mentions rendering as raw text. Kept in step with the
// backend's mentionTokenPattern.
const MENTION_TOKEN = /@\[([^\]]*)\]\((passwords|keyvault):([^)\s]{1,128})\)/g;

export type NotesSegment =
  | { kind: "text"; text: string }
  | { kind: "mention"; resourceId: string; category: string; cachedName: string };

// Splits notes into plain text and mention segments for rendering. The cached
// name is only a fallback: the live name from the resolver wins whenever the
// viewer can see the object, so renaming a target does not rot the chip.
export function parseNotesSegments(notes: string): NotesSegment[] {
  const segments: NotesSegment[] = [];
  let lastIndex = 0;
  for (const match of notes.matchAll(MENTION_TOKEN)) {
    const start = match.index ?? 0;
    if (start > lastIndex) {
      segments.push({ kind: "text", text: notes.slice(lastIndex, start) });
    }
    segments.push({
      kind: "mention",
      cachedName: match[1],
      category: match[2],
      // Not lowercased — ids are opaque and matched exactly.
      resourceId: match[3]
    });
    lastIndex = start + match[0].length;
  }
  if (lastIndex < notes.length) {
    segments.push({ kind: "text", text: notes.slice(lastIndex) });
  }
  return segments;
}

export function mentionIdsIn(notes: string): string[] {
  const ids = parseNotesSegments(notes)
    .filter((segment): segment is Extract<NotesSegment, { kind: "mention" }> => segment.kind === "mention")
    .map((segment) => segment.resourceId);
  return Array.from(new Set(ids));
}

// The name is display-only, so stripping the one character that would make the
// token ambiguous is lossless in every way that matters.
export function buildMentionToken(candidate: MentionCandidate): string {
  const safeName = candidate.name.replace(/[[\]]/g, "").trim() || "object";
  return `@[${safeName}](${candidate.category}:${candidate.resourceId})`;
}

// Finds an in-progress "@query" at the caret so the picker can open while typing
// and filter as more is typed. Returns null unless the caret sits directly at
// the end of such a fragment — that is what stops an "@" earlier in the note
// from re-triggering the picker.
export function activeMentionQuery(value: string, caret: number): { start: number; query: string } | null {
  const upToCaret = value.slice(0, caret);
  const at = upToCaret.lastIndexOf("@");
  if (at < 0) {
    return null;
  }
  // Must start a word: beginning of input, or preceded by whitespace.
  if (at > 0 && !/\s/.test(upToCaret[at - 1])) {
    return null;
  }
  const query = upToCaret.slice(at + 1);
  // A newline or a completed token ends the fragment.
  if (/[\n\]()]/.test(query)) {
    return null;
  }
  return { start: at, query };
}

export function mentionTargetFor(
  targets: MentionTarget[],
  resourceId: string
): MentionTarget | undefined {
  return targets.find((target) => target.resourceId === resourceId);
}
