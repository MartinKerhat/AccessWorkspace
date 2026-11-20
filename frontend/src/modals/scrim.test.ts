import { describe, expect, it, vi } from "vitest";
import type { MouseEvent } from "react";
import { scrimDismissProps } from "./scrim";

// A stand-in for React's synthetic event: only target/currentTarget matter.
function scrimEvent(onScrim: boolean) {
  const scrim = {};
  return { target: onScrim ? scrim : {}, currentTarget: scrim } as unknown as MouseEvent<HTMLDivElement>;
}

function clickScrim(props: ReturnType<typeof scrimDismissProps>, pressOnScrim = true) {
  props.onMouseDown(scrimEvent(pressOnScrim));
  props.onClick(scrimEvent(true));
}

describe("scrimDismissProps", () => {
  it("closes on a click on the scrim", () => {
    const onDismiss = vi.fn();
    clickScrim(scrimDismissProps(onDismiss));
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it("ignores a drag that started inside the card", () => {
    const onDismiss = vi.fn();
    clickScrim(scrimDismissProps(onDismiss), false);
    expect(onDismiss).not.toHaveBeenCalled();
  });

  it("ignores clicks on the scrim while locked (form has unsaved changes)", () => {
    const onDismiss = vi.fn();
    clickScrim(scrimDismissProps(onDismiss, { locked: true }));
    expect(onDismiss).not.toHaveBeenCalled();
  });

  it("closes again once unlocked (changes reverted)", () => {
    const onDismiss = vi.fn();
    clickScrim(scrimDismissProps(onDismiss, { locked: true }));
    clickScrim(scrimDismissProps(onDismiss, { locked: false }));
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});
