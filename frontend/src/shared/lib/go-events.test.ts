import { Events } from "@wailsio/runtime";
import { describe, expect, it, vi } from "vitest";

import { shareEventDispatch } from "./go-events";

/** What Go evaluates in the page for every event it emits. */
const fromGo = (name: string, data: unknown) => window._wails!.dispatchWailsEvent!({ name, data });

describe("shareEventDispatch", () => {
  it("keeps Go's events reaching the app after another runtime copy loads", () => {
    const seen: string[] = [];
    Events.On("sync:done", ({ data }) => seen.push(`app ${JSON.stringify(data)}`));
    shareEventDispatch();
    fromGo("sync:done", { ok: true });

    // A second copy of the runtime (`import('/wails/runtime.js')`) sets its
    // own dispatcher as it loads, as @wailsio/runtime's events.js does.
    const other = vi.fn((e: { name: string }) => seen.push(`other ${e.name}`));
    window._wails!.dispatchWailsEvent = other;
    fromGo("sync:done", { ok: false });

    expect(seen).toEqual(['app {"ok":true}', 'app {"ok":false}', "other sync:done"]);

    // Loading the same copy again adds nothing.
    window._wails!.dispatchWailsEvent = other;
    fromGo("sync:done", 1);
    expect(other).toHaveBeenCalledTimes(2);
  });
});
