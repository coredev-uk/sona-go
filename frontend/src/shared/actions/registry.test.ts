import { Music } from "lucide-react";
import { describe, expect, it } from "vitest";

import { actionsFor, menuSections, registerActions, type ActionDef } from "./registry";

const song = (id: string, group: ActionDef<"song">["group"], order: number): ActionDef<"song"> => ({
  id,
  target: "song",
  label: id,
  icon: Music,
  group,
  order,
  run: () => {},
});

describe("action registry", () => {
  it("lists a target's actions by group, then order, in non-empty sections", () => {
    registerActions([
      song("love", "rating", 10),
      song("add", "item", 30),
      song("later", "queue", 20),
      song("next", "queue", 10),
    ]);
    expect(actionsFor("song").map((a) => a.id)).toEqual(["next", "later", "add", "love"]);
    expect(actionsFor("album")).toEqual([]);

    const sections = menuSections(actionsFor("song").filter((a) => a.id !== "add"));
    expect(sections.map((s) => [s.group, s.actions.map((a) => a.id)])).toEqual([
      ["queue", ["next", "later"]],
      ["rating", ["love"]],
    ]);
  });

  it("rejects an id registered twice", () => {
    expect(() => registerActions([song("dup", "queue", 1), song("dup", "queue", 2)])).toThrow(
      "dup",
    );
  });
});
