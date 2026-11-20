import { useEffect, useRef, type MouseEvent } from "react";

// Dismiss-on-scrim-click, without eating text selections.
//
// The bug this fixes: selecting text inside a modal and releasing the mouse
// outside it closed the modal and threw the edit away. `stopPropagation` on the
// card cannot prevent it — for a drag, the browser fires `click` on the nearest
// common ancestor of the mousedown and mouseup targets, which for card→scrim is
// the scrim itself. So the click genuinely originates there and looks identical
// to a deliberate click on empty space.
//
// The distinguishing signal is where the press STARTED. Close only when both the
// press and the click land on the scrim; a drag that began inside the card is a
// selection and is ignored. This keeps click-outside working on every edge,
// which a "left side does not close" rule would not.
//
// `locked` turns click-outside off entirely. Form modals pass their
// unsaved-changes flag: once something has been typed or toggled, a stray click
// next to the card must not throw it away. The Close button and Esc still close,
// because those are deliberate.
//
// The flag is module-level on purpose: there is one pointer, and at most one
// modal is being interacted with at a time. It also keeps this a plain function
// rather than a hook, so it can be called inline in JSX without constraining
// where a component may return early.
let pressStartedOnScrim = false;

export function scrimDismissProps(onDismiss: () => void, options: { locked?: boolean } = {}) {
  return {
    onMouseDown: (event: MouseEvent<HTMLDivElement>) => {
      pressStartedOnScrim = event.target === event.currentTarget;
    },
    onClick: (event: MouseEvent<HTMLDivElement>) => {
      // Clicks that land on the card are not ours to act on.
      if (event.target !== event.currentTarget) {
        return;
      }
      if (!pressStartedOnScrim) {
        return;
      }
      pressStartedOnScrim = false;
      if (options.locked) {
        return;
      }
      onDismiss();
    }
  };
}

// True once `value` differs from what it was on the first render. Compared by
// JSON so a field edited and then changed back counts as unchanged again —
// click-outside comes back with it. Meant for modals that create their form
// state on mount; a form that loads its starting values in an effect after
// mount tracks its own baseline instead (see ResourceFormCard).
export function useUnsavedChanges(value: unknown): boolean {
  const current = JSON.stringify(value);
  const baseline = useRef<string | null>(null);
  if (baseline.current === null) {
    baseline.current = current;
  }
  return current !== baseline.current;
}

// Esc closes the top-most open modal — and only that one, so Esc in the
// mention picker opened from the object form closes the picker and leaves the
// form open. Modals register in mount order, which is also their stacking
// order: a nested modal always mounts after the one it was opened from.
//
// Anything inside the modal that has its own Esc behaviour (an open dropdown,
// the @-mention suggestions) claims the key with `preventDefault()`, and this
// handler then leaves the modal alone. React dispatches its handlers at the
// app root, which sits below `document`, so they always run before this one.
const escapeStack: symbol[] = [];

export function useModalEscape(onClose: () => void) {
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  useEffect(() => {
    const id = Symbol("modal");
    escapeStack.push(id);
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented || event.isComposing) {
        return;
      }
      if (escapeStack[escapeStack.length - 1] !== id) {
        return;
      }
      event.preventDefault();
      onCloseRef.current();
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      const index = escapeStack.indexOf(id);
      if (index >= 0) {
        escapeStack.splice(index, 1);
      }
    };
  }, []);
}
