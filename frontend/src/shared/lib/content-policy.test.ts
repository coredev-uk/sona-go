import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { useContentPolicy, useWithheld } from "./content-policy";

describe("content policy", () => {
  afterEach(() => useContentPolicy.getState().setRestricted(false));

  it("withholds explicit items only on restricted accounts", () => {
    const explicit = renderHook(() => useWithheld(true));
    const clean = renderHook(() => useWithheld(false));
    expect(explicit.result.current).toBe(false);

    act(() => useContentPolicy.getState().setRestricted(true));
    explicit.rerender();
    clean.rerender();
    expect(explicit.result.current).toBe(true);
    expect(clean.result.current).toBe(false);
  });
});
