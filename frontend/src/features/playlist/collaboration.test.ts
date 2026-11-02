import type { PlaylistCollaboration, SocialProfile, Song } from "@bindings/services";
import { describe, expect, it } from "vitest";

import { addedByRows } from "./collaboration";

const person = (id: string) => ({ id, name: id }) as SocialProfile;
const song = (id: string, catalogId = id) => ({ id, catalogId }) as Song;

const collab = {
  collaborative: true,
  ownerId: "robin",
  collaborators: [person("robin"), person("ada")],
  former: [person("jules")],
  addedBy: [
    { songId: "1", profileId: "ada" },
    { songId: "2", profileId: "" },
    { songId: "3", profileId: "jules" },
    { songId: "1", profileId: "robin" },
  ],
} as PlaylistCollaboration;

const names = (rows: (SocialProfile | undefined)[]) => rows.map((p) => p?.id ?? "-");

describe("addedByRows", () => {
  it("attributes rows in playlist order, a repeated song to each adder", () => {
    const rows = addedByRows([song("1"), song("2"), song("3"), song("1")], collab);
    expect(names(rows)).toEqual(["ada", "-", "jules", "robin"]);
  });

  it("matches library songs by catalogue ID, whatever their order", () => {
    const rows = addedByRows([song("i.c", "3"), song("i.a", "1"), song("i.b", "1")], collab);
    expect(names(rows)).toEqual(["jules", "ada", "robin"]);
  });

  it("leaves rows Apple did not attribute empty", () => {
    const rows = addedByRows(
      [song("1"), song("1"), song("1"), song("9"), song("i.up", "")],
      collab,
    );
    expect(names(rows)).toEqual(["ada", "robin", "-", "-", "-"]);
  });
});
