import type { MouseEvent } from "react";

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
// The flag is module-level on purpose: there is one pointer, and at most one
// modal is being interacted with at a time. It also keeps this a plain function
// rather than a hook, so it can be called inline in JSX without constraining
// where a component may return early.
let pressStartedOnScrim = false;

export function scrimDismissProps(onDismiss: () => void) {
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
      onDismiss();
    }
  };
}
