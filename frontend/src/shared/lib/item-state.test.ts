import type { ItemRef } from "@bindings/services";
import { QueryClient } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { bindItemStates, changeItemState, loadItemState } from "./item-state";
import { keys } from "./keys";

const { States, Favorite } = vi.hoisted(() => ({
  States: vi.fn(async (refs: ItemRef[]) =>
    refs.map((r) => ({
      kind: r.kind,
      id: r.id,
      inLibrary: r.id === "1",
      libraryId: "",
      rating: 0,
    })),
  ),
  Favorite: vi.fn(),
}));

vi.mock("@bindings/services", () => ({
  Source: { SourceCatalog: "catalog", SourceLibrary: "library" },
  LibraryService: { States, Favorite },
}));

const song = (id: string) => ({ id, source: "catalog" as never, catalogId: id });

describe("item states", () => {
  beforeEach(() => {
    States.mockClear();
    Favorite.mockReset();
  });

  it("batches every load made in the same frame into one call", async () => {
    const [a, b, c] = await Promise.all([
      loadItemState("songs", song("1")),
      loadItemState("albums", { id: "l.2", source: "library" as never, catalogId: "2" }),
      loadItemState("songs", song("3")),
    ]);
    expect(States).toHaveBeenCalledTimes(1);
    expect(States.mock.calls[0][0]).toEqual([
      { kind: "songs", id: "1", source: "catalog", catalogId: "1", libraryId: "" },
      { kind: "albums", id: "l.2", source: "library", catalogId: "2", libraryId: "" },
      { kind: "songs", id: "3", source: "catalog", catalogId: "3", libraryId: "" },
    ]);
    expect([a.inLibrary, b.id, c.id]).toEqual([true, "l.2", "3"]);

    await loadItemState("songs", song("4"));
    expect(States).toHaveBeenCalledTimes(2);
  });

  it("shows a change at once and restores the old state when Go refuses it", async () => {
    const qc = new QueryClient();
    bindItemStates(qc);
    const key = keys.itemState("songs", "7");
    const before = { kind: "songs", id: "7", inLibrary: false, libraryId: "", rating: 0 };
    qc.setQueryData(key, before);

    let settle!: (v: unknown) => void;
    Favorite.mockReturnValueOnce(new Promise((resolve) => (settle = resolve)));
    const done = changeItemState("songs", song("7"), "favorite");
    await vi.waitFor(() => expect(qc.getQueryData(key)).toMatchObject({ rating: 1 }));
    settle({ ...before, rating: 1, inLibrary: true, libraryId: "i.7" });
    await expect(done).resolves.toMatchObject({ inLibrary: true, libraryId: "i.7" });
    expect(qc.getQueryData(key)).toMatchObject({ rating: 1, libraryId: "i.7" });

    Favorite.mockRejectedValueOnce(new Error("offline"));
    await expect(changeItemState("songs", song("7"), "unfavorite")).rejects.toThrow("offline");
    expect(qc.getQueryData(key)).toMatchObject({ rating: 1, libraryId: "i.7" });
  });
});
