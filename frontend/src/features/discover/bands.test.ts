import type { Item, Section } from "@bindings/services";
import { describe, expect, it } from "vitest";

import { toBands } from "./bands";

function item(kind: string, url = ""): Item {
  return {
    kind,
    id: kind,
    name: kind,
    subtitle: "",
    caption: "",
    url,
    color: "",
    explicit: false,
    art: {
      small: "",
      medium: url ? "/art/0123456789abcdef0123456789abcdef/320x180.jpg" : "",
      large: "",
    },
    wide: { small: "", medium: "", large: "" },
  } as unknown as Item;
}

function section(id: string, kind: string, kinds: string[] = [], title = id): Section {
  return { id, kind, title, text: "", items: kinds.map((k) => item(k)), room: "", seeAll: false };
}

const ids = (sections: readonly Section[]) => sections.map((s) => s.id);

describe("toBands", () => {
  it("groups shelves by what they hold, in the bands' order, keeping Apple's order inside each", () => {
    const bands = toBands([
      section("tracks", "tracks", ["song"]),
      section("playlists", "shelf", ["playlist", "playlist"]),
      section("albums", "shelf", ["album", "album"]),
      section("bricks", "bricks", ["curator"]),
      section("live", "live", ["station"]),
      section("episodes", "shelf", ["station"]),
      section("videos", "shelf", ["music-video"]),
      section("links", "links", ["grouping"]),
    ]);
    expect(bands.map((b) => [b.id, ids(b.sections)])).toEqual([
      ["music", ["tracks", "albums"]],
      ["playlists", ["playlists"]],
      ["radio", ["live", "episodes"]],
      ["video", ["videos"]],
      ["browse", ["bricks", "links"]],
    ]);
  });

  it("files a mixed shelf by most of its items, and a tie by its first item", () => {
    const [mostly] = toBands([section("mixed", "shelf", ["album", "playlist", "playlist"])]);
    expect(mostly.id).toBe("playlists");
    const [tie] = toBands([section("tie", "shelf", ["album", "playlist"])]);
    expect(tie.id).toBe("music");
  });

  it("counts a web link to a video as a video, and any other web link as browsing", () => {
    const interviews = section("interviews", "shelf");
    interviews.items = [item("link", "https://music.apple.com/gb/post/123")];
    const concerts = section("concerts", "shelf");
    concerts.items = [item("link", "https://music.apple.com/gb/concerts")];
    expect(toBands([interviews, concerts]).map((b) => [b.id, ids(b.sections)])).toEqual([
      ["video", ["interviews"]],
      ["browse", ["concerts"]],
    ]);
  });

  it("keeps a paragraph with the section it introduces", () => {
    const bands = toBands([
      section("albums", "shelf", ["album"]),
      section("intro", "text"),
      section("playlists", "shelf", ["playlist"]),
      section("outro", "text"),
    ]);
    expect(bands.map((b) => ids(b.sections))).toEqual([
      ["albums"],
      ["intro", "playlists", "outro"],
    ]);
  });
});
