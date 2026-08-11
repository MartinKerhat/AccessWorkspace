import { mentionTargetFor, parseNotesSegments } from "../mentions";
import type { MentionTarget } from "../types";

type Props = {
  notes: string;
  targets: MentionTarget[];
  onOpenMention: (target: MentionTarget) => void;
};

// Renders notes with object mentions as clickable chips. Everything else stays
// literal text, including a stray "@" or a hand-mangled token.
//
// A mention the viewer may not open still renders as a chip and discloses the
// name — deliberate, so a broadly shared connection can say which credential is
// relevant without handing it over. A "hidden" target (personal, archived or
// gone) discloses nothing and falls back to plain text.
export function MentionNotes({ notes, targets, onOpenMention }: Props) {
  if (!notes) {
    return <p className="connection-notes-copy">n/a</p>;
  }

  return (
    <p className="connection-notes-copy">
      {parseNotesSegments(notes).map((segment, index) => {
        if (segment.kind === "text") {
          return <span key={index}>{segment.text}</span>;
        }

        const target = mentionTargetFor(targets, segment.resourceId);

        // Not resolved yet, or nothing may be disclosed: plain text.
        if (!target || target.state === "hidden") {
          return <span key={index}>{target ? "" : segment.cachedName}</span>;
        }

        // Live name wins over the cached copy, so a rename does not rot the chip.
        const label = target.name || segment.cachedName;
        const denied = target.state === "denied";
        return (
          <button
            key={index}
            type="button"
            className={`mention-chip${denied ? " denied" : ""}`}
            onClick={() => onOpenMention(target)}
            title={denied ? `${label} — you do not have access to this object` : label}
          >
            {label}
          </button>
        );
      })}
    </p>
  );
}
