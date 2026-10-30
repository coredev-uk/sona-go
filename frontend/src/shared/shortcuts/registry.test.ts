import { describe, expect, it } from "vitest";

import type { Shortcut } from "@/shared/module";

import { conflictingIds, conflictsFor, rebind, resolveBindings } from "./registry";

const sc = (id: string, keys: string[], scope?: string): Shortcut => ({
  id,
  label: () => id,
  group: "general",
  keys,
  scope,
  run: () => {},
});

const list = [
  sc("player.toggle", ["Space"]),
  sc("player.next", ["Ctrl+ArrowRight"]),
  sc("search.focus", ["/", "Ctrl+F"], "global"),
  sc("player.close", ["Escape"], "now-playing"),
  sc("lyrics.close", ["Escape"], "lyrics-room"),
  sc("player.favourite", ["F"]),
];

describe("resolveBindings", () => {
  it("uses an override over the defaults", () => {
    const b = resolveBindings(list, { "player.toggle": "K", "search.focus": "Ctrl+Shift+F" });
    expect(b.get("player.toggle")).toEqual(["K"]);
    expect(b.get("search.focus")).toEqual(["Ctrl+Shift+F"]);
    expect(b.get("player.next")).toEqual(["Ctrl+ArrowRight"]);
  });

  it("unbinds with none and keeps defaults for blank or invalid overrides", () => {
    const b = resolveBindings(list, {
      "player.toggle": "none",
      "player.next": "",
      "search.focus": "Hyper+F",
    });
    expect(b.get("player.toggle")).toEqual([]);
    expect(b.get("player.next")).toEqual(["Ctrl+ArrowRight"]);
    expect(b.get("search.focus")).toEqual(["/", "Ctrl+F"]);
  });

  it("ignores overrides for shortcuts that are not registered", () => {
    expect(resolveBindings(list, { "gone.action": "Ctrl+K" }).has("gone.action")).toBe(false);
  });
});

describe("conflicts", () => {
  it("finds a chord used twice in overlapping scopes", () => {
    const b = resolveBindings(list, { "player.favourite": "/" });
    expect(conflictsFor(list, b, "player.favourite", "/").map((s) => s.id)).toEqual([
      "search.focus",
    ]);
    expect(conflictingIds(list, b)).toEqual(new Set(["player.favourite", "search.focus"]));
  });

  it("allows one chord in distinct contexts", () => {
    const b = resolveBindings(list, {});
    expect(conflictsFor(list, b, "player.close", "Escape")).toEqual([]);
    expect(conflictingIds(list, b).size).toBe(0);
  });

  it("counts a context against page-wide keys", () => {
    const b = resolveBindings(list, { "player.close": "Space" });
    expect(conflictsFor(list, b, "player.close", "Space").map((s) => s.id)).toEqual([
      "player.toggle",
    ]);
  });
});

describe("rebind", () => {
  it("stores only bindings that differ from the defaults", () => {
    expect(rebind(list, {}, "player.toggle", ["K"])).toEqual({ "player.toggle": "K" });
    expect(rebind(list, { "player.toggle": "K" }, "player.toggle", ["Space"])).toEqual({});
    expect(rebind(list, {}, "player.toggle", [])).toEqual({ "player.toggle": "none" });
  });

  it("moves a stolen chord, leaving the other shortcut's other keys", () => {
    const next = rebind(list, {}, "player.favourite", ["Ctrl+F"], true);
    expect(next).toEqual({ "player.favourite": "Ctrl+F", "search.focus": "/" });
    const b = resolveBindings(list, next);
    expect(conflictingIds(list, b).size).toBe(0);
  });

  it("leaves conflicts in place without steal", () => {
    const next = rebind(list, {}, "player.favourite", ["Space"]);
    expect(conflictingIds(list, resolveBindings(list, next))).toEqual(
      new Set(["player.favourite", "player.toggle"]),
    );
  });
});
