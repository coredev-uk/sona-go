import type { Album, Artist, Item, Playlist, Station } from "@bindings/services";
import { act, cleanup, fireEvent, render } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";
import { toCoverItem } from "@/shared/items/cover";
import { MotionContext, type MotionState, TILE_DWELL_MS, useTileMotion } from "@/shared/lib/motion";
import { albumTile, artistTile, playlistTile } from "@/shared/lib/tiles";

import type { CoverItem } from "./CoverGrid";
import { CoverRow } from "./CoverRow";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, "data-opener": opener }: { children: ReactNode; "data-opener"?: boolean }) => (
    <a href="/" data-opener={opener}>
      {children}
    </a>
  ),
}));
vi.mock("@/shared/actions/ActionMenu", () => ({ ItemActionMenu: () => null }));
// The real video fetches the file and plays it; here only which tile
// mounts it, and for which URL, matters.
vi.mock("@/shared/components/MotionCover", () => ({
  MotionVideo: ({ src }: { src: string }) => <video data-testid="motion" data-src={src} />,
}));

beforeAll(() => {
  globalThis.ResizeObserver ??= class {
    observe() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});
beforeEach(() => {
  vi.useFakeTimers();
  useTileMotion.setState({ active: null });
});
afterEach(() => {
  // The pointer leaves the window: the delegated listeners forget the tile.
  fireEvent.pointerOut(document.body);
  if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
  cleanup();
  vi.useRealTimers();
});

const ALLOWED: MotionState = {
  probed: true,
  allowed: true,
  setting: true,
  reduceEffects: false,
  reducedMotion: false,
  power: null,
  h264: "probably",
  hevc: "",
  codec: "avc",
};

const art = { small: "", medium: "/art/0123456789abcdef0123456789abcdef/320x320.jpg", large: "" };
const motion = (key: string) => `/motion/${key.padEnd(24, "0")}/720.mp4`;

// Catalog DTOs as the artist, album, playlist and Radio pages get them.
const essentials = {
  id: "pl.72ee689fe3f84ce2b46a6913c2e6f5f1",
  name: "Elton John Essentials",
  curatorName: "Apple Music Pop",
  art,
  motion: motion("a1"),
} as unknown as Playlist;
const rocketHour = {
  id: "pl.9192e00c1da741719f7b4816d164031e",
  name: "Elton John’s Rocket Hour",
  curatorName: "Apple Music",
  art,
  motion: "",
} as unknown as Playlist;
const album = {
  id: "1440863013",
  name: "Goodbye Yellow Brick Road",
  artistName: "Elton John",
  releaseDate: "1973",
  art,
  motion: motion("b2"),
} as unknown as Album;
const artist = {
  id: "1031397873",
  name: "Dua Lipa",
  art,
  motion: motion("c3"),
} as unknown as Artist;
const station = {
  id: "ra.u-1",
  name: "Your Station",
  art,
  motion: motion("d4"),
  playable: true,
} as unknown as Station;

function renderRow(items: CoverItem[]) {
  return render(
    <MotionContext value={ALLOWED}>
      {/* jsdom has no layout: one column, so one row per tile. */}
      <CoverRow items={items} label="Row" rows={items.length} />
    </MotionContext>,
    { wrapper: I18nRoot },
  );
}

function tile(container: HTMLElement, title: string): HTMLElement {
  const t = [...container.querySelectorAll<HTMLElement>("[data-cover-tile]")].find((el) =>
    el.textContent?.includes(title),
  );
  if (!t) throw new Error(`no tile ${title}`);
  return t;
}

function hover(el: HTMLElement) {
  fireEvent.pointerOver(el.querySelector("img") ?? el);
  act(() => vi.advanceTimersByTime(TILE_DWELL_MS));
}

describe("tile motion on hover", () => {
  it("carries motion from every catalog DTO tile and Item", () => {
    expect(playlistTile(essentials).motion).toBe(essentials.motion);
    expect(playlistTile(rocketHour).motion).toBeUndefined();
    expect(albumTile(album).motion).toBe(album.motion);
    expect(artistTile(artist).motion).toBe(artist.motion);
    const item = (kind: string, field: string, v: unknown) =>
      ({ kind, id: "x", name: "x", art, wide: art, [field]: v }) as unknown as Item;
    expect(toCoverItem(item("playlist", "playlist", essentials), 0).motion).toBe(essentials.motion);
    expect(toCoverItem(item("album", "album", album), 0).motion).toBe(album.motion);
    expect(toCoverItem(item("artist", "artist", artist), 0).motion).toBe(artist.motion);
    expect(toCoverItem(item("station", "station", station), 0).motion).toBe(station.motion);
    expect(toCoverItem(item("playlist", "playlist", rocketHour), 0).motion).toBeUndefined();
  });

  it("plays a shelf tile's motion after the dwell, one tile at a time", () => {
    const { container } = renderRow([
      playlistTile(essentials),
      albumTile(album),
      artistTile(artist),
    ]);
    const ej = tile(container, "Elton John Essentials");
    expect(ej.querySelector("[data-tile-motion]")).not.toBeNull();
    expect(container.querySelector("video")).toBeNull();

    fireEvent.pointerOver(ej.querySelector("img") ?? ej);
    act(() => vi.advanceTimersByTime(TILE_DWELL_MS - 50));
    expect(container.querySelector("video")).toBeNull();
    act(() => vi.advanceTimersByTime(50));
    const v = ej.querySelector("video");
    // 180 CSS px at density 1, the tile rendition.
    expect(v?.dataset.src).toBe(motion("a1").replace("/720.mp4", "/180.mp4"));

    hover(tile(container, "Dua Lipa"));
    expect(container.querySelectorAll("video")).toHaveLength(1);
    expect(tile(container, "Dua Lipa").querySelector("video")).not.toBeNull();
  });

  it("plays on keyboard focus too", () => {
    const { container } = renderRow([albumTile(album)]);
    const t = tile(container, "Goodbye Yellow Brick Road");
    act(() => t.querySelector<HTMLElement>("[data-opener]")?.focus());
    fireEvent.focusIn(t.querySelector("[data-opener]") ?? t);
    act(() => vi.advanceTimersByTime(TILE_DWELL_MS));
    expect(t.querySelector("video")).not.toBeNull();
  });

  it("does nothing for a tile Apple has no motion for", () => {
    const { container } = renderRow([playlistTile(rocketHour)]);
    const t = tile(container, "Rocket Hour");
    expect(t.querySelector("[data-tile-motion]")).toBeNull();
    hover(t);
    expect(container.querySelector("video")).toBeNull();
  });
});
