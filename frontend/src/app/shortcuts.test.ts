import { describe, expect, it } from "vitest";

import type { Shortcut } from "@/shared/module";
import { resolveBindings } from "@/shared/shortcuts/registry";

import { chordIndex, keyAllowed } from "./shortcuts";

const idle = { typing: false, control: false };
const typing = { typing: true, control: true };
const onButton = { typing: false, control: true };

describe("keyAllowed", () => {
  it("keeps every key in a text field except global modifier chords", () => {
    expect(keyAllowed("page", "M", typing)).toBe(false);
    expect(keyAllowed("global", "/", typing)).toBe(false);
    expect(keyAllowed("global", "Ctrl+K", typing)).toBe(true);
    expect(keyAllowed("page", "Ctrl+ArrowRight", typing)).toBe(false);
  });

  it("leaves a control the plain keys it operates with", () => {
    expect(keyAllowed("page", "Space", onButton)).toBe(false);
    expect(keyAllowed("page", "Shift+ArrowLeft", onButton)).toBe(false);
    expect(keyAllowed("page", "M", onButton)).toBe(true);
    expect(keyAllowed("page", "Ctrl+ArrowRight", onButton)).toBe(true);
    expect(keyAllowed("page", "Space", idle)).toBe(true);
  });
});

describe("chordIndex", () => {
  it("indexes the user's chords, not the replaced defaults, in registration order", () => {
    const sc = (id: string, keys: string[]): Shortcut => ({
      id,
      label: () => id,
      group: "general",
      keys,
      run: () => {},
    });
    const list = [sc("a", ["Ctrl+K"]), sc("b", ["K"]), sc("c", ["K"])];
    const index = chordIndex(list, resolveBindings(list, { a: "Ctrl+P" }));
    expect(index.has("Ctrl+K")).toBe(false);
    expect(index.get("Ctrl+P")?.map((s) => s.id)).toEqual(["a"]);
    expect(index.get("K")?.map((s) => s.id)).toEqual(["b", "c"]);
  });
});
