import type { Pin, Playlist, PlaylistTree, TreePlaylist } from "@bindings/services";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";
import { TooltipProvider } from "@/shared/ui/tooltip";

import { PlaylistSidebar } from "./PlaylistSidebar";

const data = vi.hoisted(() => ({
  tree: undefined as PlaylistTree | undefined,
  pins: undefined as Pin[] | undefined,
}));

vi.mock("@bindings/services", () => ({ PlaylistsService: {} }));
vi.mock("./queries", () => ({
  usePlaylistTree: () => ({ data: data.tree, isPending: !data.tree }),
  usePins: () => ({ data: data.pins, isPending: !data.pins }),
}));
vi.mock("./drops", () => ({
  addable: () => false,
  pinnable: () => [],
  useNewPlaylistDrop: () => false,
  usePinDrop: () => false,
  usePlaylistDrop: () => false,
}));
vi.mock("./commands", () => ({ deleteFolder: vi.fn(), playPin: vi.fn(), unpin: vi.fn() }));
vi.mock("./dialogs", () => ({ showDialog: vi.fn() }));
vi.mock("@/shared/dnd/drag", () => ({
  useDrag: { getState: () => ({ active: false }) },
  useDragActive: () => false,
}));
vi.mock("@/shared/actions/ActionMenu", () => ({ ActionMenu: () => null }));
vi.mock("@/shared/components/Artwork", () => ({ Artwork: () => null }));
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, className }: { children: ReactNode; className?: string }) => (
    <a href="#/playlist" className={className}>
      {children}
    </a>
  ),
  useNavigate: () => vi.fn(),
}));

function item(
  id: string,
  name: string,
  position: number,
  canEdit: boolean,
  catalogId = "",
  folderId = "",
): TreePlaylist {
  return {
    playlist: { id, name, canEdit, catalogId, art: { small: "" } } as Playlist,
    folderId,
    position,
    canDelete: true,
    favorited: false,
  };
}

const library: PlaylistTree = {
  folders: [{ id: "f.run", parentId: "", name: "Running", position: 0 }],
  playlists: [
    item("p.own", "Road Trip", 3, true),
    item("p.mix1", "Morning Mix", 0, false, "pl.pm-1"),
    item("p.mix2", "Discovery Mix", 1, false, "pl.pm-2"),
    item("p.ed", "Quiet Hours", 4, false, "pl.abc"),
    item("p.run", "Tempo", 0, true, "", "f.run"),
  ],
};

function renderSidebar(pinned: string[] = []) {
  data.tree = library;
  data.pins = pinned.map(
    (id) => ({ kind: "playlist", id, name: id, action: "drillIn", art: { small: "" } }) as Pin,
  );
  return render(
    <TooltipProvider>
      <PlaylistSidebar />
    </TooltipProvider>,
    { wrapper: I18nRoot },
  );
}

const headings = () => screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent);
const sectionOf = (name: string) => {
  const heading = screen.getByRole("heading", { level: 2, name });
  return heading.closest("section") as HTMLElement;
};
const listed = (section: HTMLElement) =>
  within(section)
    .getAllByRole("link")
    .map((l) => l.textContent);

beforeEach(() => localStorage.clear());
afterEach(() => {
  cleanup();
  data.tree = undefined;
  data.pins = undefined;
});

describe("the sidebar's playlist sections", () => {
  it("lists Made for you, Apple Music and the user's Playlists in that order", () => {
    renderSidebar();
    expect(headings()).toEqual(["Made for you", "Apple Music", "Playlists"]);
    expect(listed(sectionOf("Made for you"))).toEqual(["Morning Mix", "Discovery Mix"]);
    expect(listed(sectionOf("Apple Music"))).toEqual(["Quiet Hours"]);
    expect(within(sectionOf("Playlists")).getByText("Road Trip")).toBeTruthy();
    expect(within(sectionOf("Playlists")).queryByText("Morning Mix")).toBeNull();
  });

  it("offers New playlist only on the user's Playlists", () => {
    renderSidebar();
    expect(screen.getAllByRole("button", { name: "New playlist or folder" })).toHaveLength(1);
    expect(
      within(sectionOf("Playlists")).getByRole("button", { name: "New playlist or folder" }),
    ).toBeTruthy();
  });

  it("shows a pinned playlist under Pinned only, and drops a section it empties", () => {
    renderSidebar(["p.mix1", "p.mix2", "p.ed"]);
    expect(headings()).toEqual(["Pinned", "Playlists"]);
    expect(screen.queryByRole("heading", { name: "Made for you" })).toBeNull();
    expect(screen.queryByRole("heading", { name: "Apple Music" })).toBeNull();
  });

  it("keeps the unpinned ones of a section in it", () => {
    renderSidebar(["p.mix1"]);
    expect(headings()).toEqual(["Pinned", "Made for you", "Apple Music", "Playlists"]);
    expect(listed(sectionOf("Made for you"))).toEqual(["Discovery Mix"]);
  });

  it("folds a section shut and remembers it", () => {
    renderSidebar();
    const fold = within(sectionOf("Apple Music")).getByRole("button", { name: "Apple Music" });
    expect(fold.getAttribute("aria-expanded")).toBe("true");
    fireEvent.click(fold);
    expect(fold.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByText("Quiet Hours")).toBeNull();
    cleanup();
    renderSidebar();
    expect(
      within(sectionOf("Apple Music"))
        .getByRole("button", { name: "Apple Music" })
        .getAttribute("aria-expanded"),
    ).toBe("false");
    expect(screen.getByText("Morning Mix")).toBeTruthy();
  });

  it("says a folder's playlists are listed in other sections", () => {
    data.tree = {
      ...library,
      playlists: [item("p.x", "Mix In Folder", 0, false, "pl.pm-9", "f.run")],
    };
    data.pins = [];
    render(
      <TooltipProvider>
        <PlaylistSidebar />
      </TooltipProvider>,
      { wrapper: I18nRoot },
    );
    fireEvent.click(screen.getByRole("button", { name: "Running" }));
    expect(screen.getByText("Its playlist is listed in another section")).toBeTruthy();
  });
});
