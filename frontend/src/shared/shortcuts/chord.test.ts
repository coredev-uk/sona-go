import { describe, expect, it } from "vitest";

import { chordParts, chordText, eventChord, isPlainChord, parseBinding, parseChord } from "./chord";

const key = (
  k: string,
  mods: Partial<Record<"ctrlKey" | "altKey" | "shiftKey" | "metaKey", boolean>> = {},
  extra: { code?: string; altGraph?: boolean } = {},
) => ({
  key: k,
  code: extra.code ?? "",
  ctrlKey: false,
  altKey: false,
  shiftKey: false,
  metaKey: false,
  ...mods,
  getModifierState: (m: string) => m === "AltGraph" && !!extra.altGraph,
});

describe("parseChord", () => {
  it("normalises modifier order, case and aliases", () => {
    expect(parseChord("shift+ctrl+k")).toBe("Ctrl+Shift+K");
    expect(parseChord("Control+Right")).toBe("Ctrl+ArrowRight");
    expect(parseChord("esc")).toBe("Escape");
    expect(parseChord(" space ")).toBe("Space");
    expect(parseChord("cmd+,")).toBe("Meta+,");
  });

  it("reads + as a key", () => {
    expect(parseChord("Ctrl++")).toBe("Ctrl+Plus");
    expect(parseChord("+")).toBe("Plus");
    expect(parseChord("Ctrl+Plus")).toBe("Ctrl+Plus");
  });

  it("drops Shift from characters the layout reaches with Shift", () => {
    expect(parseChord("Shift+?")).toBe("?");
    expect(parseChord("Ctrl+Shift+/")).toBe("Ctrl+/");
    expect(parseChord("Shift+A")).toBe("Shift+A");
    expect(parseChord("Shift+ArrowLeft")).toBe("Shift+ArrowLeft");
  });

  it("rejects what is not one chord", () => {
    for (const bad of ["", "Ctrl+", "Hyper+K", "Ctrl+Ctrl+K", "Ctrl+Nope", "K+Ctrl"]) {
      expect(parseChord(bad), bad).toBeNull();
    }
  });
});

describe("parseBinding", () => {
  it("parses several chords, deduplicated, and none", () => {
    expect(parseBinding("/ ctrl+f Ctrl+F")).toEqual(["/", "Ctrl+F"]);
    expect(parseBinding("none")).toEqual([]);
    expect(parseBinding("  ")).toBeNull();
    expect(parseBinding("Ctrl+K Nope+X")).toBeNull();
  });
});

describe("eventChord", () => {
  it("builds the canonical chord from a key event", () => {
    expect(eventChord(key("k", { ctrlKey: true }))).toBe("Ctrl+K");
    expect(eventChord(key("K", { ctrlKey: true, shiftKey: true }))).toBe("Ctrl+Shift+K");
    expect(eventChord(key(" "))).toBe("Space");
    expect(eventChord(key("ArrowRight", { shiftKey: true }))).toBe("Shift+ArrowRight");
  });

  it("matches ? however the layout reaches it", () => {
    expect(eventChord(key("?", { shiftKey: true }))).toBe(parseChord("?"));
    expect(eventChord(key("?"))).toBe("?");
  });

  it("ignores lone modifiers", () => {
    for (const k of ["Control", "Shift", "Alt", "Meta", "AltGraph", "Dead"]) {
      expect(eventChord(key(k, { ctrlKey: k === "Control" })), k).toBeNull();
    }
  });

  it("treats AltGr characters as the character, not Ctrl+Alt", () => {
    expect(eventChord(key("@", { ctrlKey: true, altKey: true }, { altGraph: true }))).toBe("@");
  });

  it("uses the physical Latin letter for Ctrl on a non-Latin layout", () => {
    expect(eventChord(key("л", { ctrlKey: true }, { code: "KeyK" }))).toBe("Ctrl+K");
    // Typing a non-Latin letter on its own stays that letter.
    expect(eventChord(key("л", {}, { code: "KeyK" }))).toBe("Л");
  });
});

describe("display", () => {
  it("shows keycaps and speaks words", () => {
    expect(chordParts("Ctrl+ArrowLeft")).toEqual(["Ctrl", "←"]);
    expect(chordParts("Ctrl+Plus")).toEqual(["Ctrl", "+"]);
    expect(chordText("Ctrl+ArrowLeft")).toBe("Ctrl+Left Arrow");
    expect(chordText("?")).toBe("Question mark");
  });

  it("knows plain chords", () => {
    expect(isPlainChord("Space")).toBe(true);
    expect(isPlainChord("Shift+ArrowLeft")).toBe(true);
    expect(isPlainChord("Ctrl+K")).toBe(false);
    expect(isPlainChord("Alt+1")).toBe(false);
  });
});
