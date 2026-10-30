import { act, render } from "@testing-library/react";
import { useEffect, useRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useReturnFocus } from "./useReturnFocus";

/** A panel that takes focus when it opens, as the now-playing view does. */
function Panel() {
  const ref = useRef<HTMLButtonElement>(null);
  useEffect(() => ref.current?.focus(), []);
  return <button ref={ref}>close</button>;
}

function Host({ open }: { open: boolean }) {
  useReturnFocus(open);
  return (
    <>
      <button>opener</button>
      <button>elsewhere</button>
      {open && <Panel />}
    </>
  );
}

const byText = (c: HTMLElement, text: string) =>
  [...c.querySelectorAll("button")].find((b) => b.textContent === text)!;

afterEach(() => vi.useRealTimers());

describe("useReturnFocus", () => {
  it("returns focus to the opener when the panel closes with focus inside", () => {
    vi.useFakeTimers();
    const { container, rerender } = render(<Host open={false} />);
    byText(container, "opener").focus();
    rerender(<Host open />);
    expect(document.activeElement).toBe(byText(container, "close"));
    rerender(<Host open={false} />);
    act(() => vi.runAllTimers());
    expect(document.activeElement).toBe(byText(container, "opener"));
  });

  it("leaves focus alone when the user moved it elsewhere", () => {
    vi.useFakeTimers();
    const { container, rerender } = render(<Host open={false} />);
    byText(container, "opener").focus();
    rerender(<Host open />);
    byText(container, "elsewhere").focus();
    rerender(<Host open={false} />);
    act(() => vi.runAllTimers());
    expect(document.activeElement).toBe(byText(container, "elsewhere"));
  });
});
