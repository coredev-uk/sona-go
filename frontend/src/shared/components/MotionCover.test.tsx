import { act, cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { MotionVideo } from "./MotionCover";

// jsdom has neither IntersectionObserver nor media playback, and is not visible.
let intersect: (visible: boolean) => void = () => {};
const play = vi.fn(() => Promise.resolve());
const pause = vi.fn();

beforeEach(() => {
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      constructor(cb: IntersectionObserverCallback) {
        intersect = (visible) =>
          cb(
            [{ isIntersecting: visible } as IntersectionObserverEntry],
            this as unknown as IntersectionObserver,
          );
      }
      observe() {}
      disconnect() {}
    },
  );
  vi.stubGlobal(
    "fetch",
    vi.fn(() => Promise.resolve(new Response(new Blob(["mp4"], { type: "video/mp4" })))),
  );
  vi.spyOn(HTMLMediaElement.prototype, "play").mockImplementation(play);
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(pause);
  vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  play.mockClear();
  pause.mockClear();
});

describe("MotionVideo", () => {
  it("pauses while its page is inert under an overlay or a modal", async () => {
    const { container } = render(
      <main>
        <MotionVideo src="/motion/0123456789abcdef01234567/200.mp4" />
      </main>,
    );
    await waitFor(() => expect(container.querySelector("video")).not.toBeNull());
    act(() => intersect(true));
    expect(play).toHaveBeenCalledTimes(1);

    const main = container.querySelector("main")!;
    await act(async () => main.setAttribute("inert", ""));
    expect(pause).toHaveBeenCalledTimes(1);

    await act(async () => main.removeAttribute("inert"));
    expect(play).toHaveBeenCalledTimes(2);
  });
});
