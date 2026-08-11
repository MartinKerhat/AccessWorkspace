import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import { activeMentionQuery, buildMentionToken } from "../mentions";
import type { MentionCandidate } from "../types";

type Props = {
  value: string;
  rows?: number;
  onChange: (value: string) => void;
};

// Notes editor with "@" object mentions. Deliberately a plain <textarea>: the
// picker inserts a token the author can see but never has to type, which avoids
// a rich-text storage format and any migration of existing notes. The trade-off
// is that the raw token is visible while editing — if that reads badly, the next
// step is a contenteditable version where it is hidden. See §5.3 of
// .dev-notes/rdp-credential-handoff-plan.md.
export function MentionNotesField({ value, rows = 3, onChange }: Props) {
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const [fragment, setFragment] = useState<{ start: number; query: string } | null>(null);
  const [candidates, setCandidates] = useState<MentionCandidate[]>([]);
  const [loading, setLoading] = useState(false);

  // Candidates come from the server per keystroke-ish, scoped to this user's own
  // visibility. Debounced so typing a long name is not one request per letter.
  useEffect(() => {
    if (!fragment) {
      setCandidates([]);
      return;
    }
    let cancelled = false;
    setLoading(true);
    const timer = window.setTimeout(async () => {
      try {
        const response = await api.listMentionCandidates(fragment.query);
        if (!cancelled) {
          setCandidates(response.items);
        }
      } catch {
        if (!cancelled) {
          setCandidates([]);
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }, 180);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [fragment?.query, Boolean(fragment)]);

  function syncFragment(nextValue: string, caret: number) {
    setFragment(activeMentionQuery(nextValue, caret));
  }

  function insert(candidate: MentionCandidate) {
    if (!fragment) {
      return;
    }
    const textarea = textareaRef.current;
    const caret = textarea?.selectionStart ?? value.length;
    const token = buildMentionToken(candidate);
    const next = `${value.slice(0, fragment.start)}${token} ${value.slice(caret)}`;
    onChange(next);
    setFragment(null);
    // Put the caret after the inserted token so typing continues naturally.
    const caretAfter = fragment.start + token.length + 1;
    window.requestAnimationFrame(() => {
      textarea?.focus();
      textarea?.setSelectionRange(caretAfter, caretAfter);
    });
  }

  return (
    <div className="picker-shell picker-shell-inline">
      <textarea
        ref={textareaRef}
        value={value}
        rows={rows}
        onChange={(event) => {
          onChange(event.target.value);
          syncFragment(event.target.value, event.target.selectionStart ?? 0);
        }}
        onClick={(event) => syncFragment(value, event.currentTarget.selectionStart ?? 0)}
        onKeyUp={(event) => {
          if (event.key === "Escape") {
            setFragment(null);
            return;
          }
          syncFragment(value, event.currentTarget.selectionStart ?? 0);
        }}
        onBlur={() => window.setTimeout(() => setFragment(null), 120)}
      />
      <p className="selection-hint">Type @ to mention a shared password or Key Vault secret.</p>
      {fragment ? (
        <div className="group-picker-dropdown">
          <div className="picker-option-list">
            {loading && candidates.length === 0 ? <span className="selection-hint">Searching…</span> : null}
            {!loading && candidates.length === 0 ? (
              <span className="selection-hint">
                {fragment.query
                  ? `Nothing you can see matches "${fragment.query}".`
                  : "No shared passwords or Key Vault secrets are available to you."}
              </span>
            ) : null}
            {candidates.map((candidate) => (
              <button
                key={candidate.resourceId}
                type="button"
                className="picker-option"
                // onMouseDown, not onClick: blur fires first and would close the
                // dropdown before a click could land.
                onMouseDown={(event) => {
                  event.preventDefault();
                  insert(candidate);
                }}
              >
                <strong>{candidate.name}</strong>
                <span>
                  {candidate.category === "keyvault" ? "Key Vault" : "Password"}
                  {candidate.username ? ` · ${candidate.username}` : ""}
                  {candidate.folderPath ? ` · ${candidate.folderPath}` : ""}
                </span>
              </button>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
}
