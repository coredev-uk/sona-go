import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useMouseNavigation } from "./useMouseNavigation";

const hoisted = vi.hoisted(() => ({
  history: { back: vi.fn(), forward: vi.fn() },
  handlers: new Map<string, (e: { data: unknown }) => void>(),
  off: vi.fn(),
}));

vi.mock("@tanstack/react-router", () => ({ useRouter: () => ({ history: hoisted.history }) }));
vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, fn: (e: { data: unknown }) => void) => {
      hoisted.handlers.set(name, fn);
      return hoisted.off;
    },
  },
}));

describe("useMouseNavigation", () => {
  beforeEach(() => {
    hoisted.history.back.mockClear();
    hoisted.history.forward.mockClear();
    hoisted.off.mockClear();
  });

  it("goes back and forward as Go says the mouse buttons were pressed", () => {
    const { unmount } = renderHook(() => useMouseNavigation());
    const send = hoisted.handlers.get("window:navigate")!;
    send({ data: "back" });
    expect(hoisted.history.back).toHaveBeenCalledTimes(1);
    expect(hoisted.history.forward).not.toHaveBeenCalled();
    send({ data: "forward" });
    expect(hoisted.history.forward).toHaveBeenCalledTimes(1);
    expect(hoisted.history.back).toHaveBeenCalledTimes(1);
    unmount();
    expect(hoisted.off).toHaveBeenCalled();
  });
});
