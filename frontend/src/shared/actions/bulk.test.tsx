import type { Album, Song } from "@bindings/services";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { ListEnd, Star } from "lucide-react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";
import { createSelectionScope, SelectionContext } from "@/shared/selection/store";
import { clickRow, emptySelection } from "@/shared/selection/model";

import { ContextActionMenu } from "./ActionMenu";
import {
  bulkActionsFor,
  describeItems,
  registerActions,
  type ActionItem,
  type BulkActionContext,
} from "./registry";

vi.mock("@tanstack/react-router", () => ({ useNavigate: () => () => {} }));

const song = (id: string, name = id): ActionItem =>
  ({ kind: "song", target: { id, name } as Song }) as ActionItem;
const album = (id: string): ActionItem =>
  ({ kind: "album", target: { id, name: id } as Album }) as ActionItem;

const queued = vi.fn<(items: readonly ActionItem[], ctx: BulkActionContext) => void>();
const single = vi.fn<(s: Song) => void>();

beforeAll(() => {
  // Radix's popper measures with ResizeObserver, which jsdom lacks.
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
  registerActions([
    {
      id: "t.queue.one",
      target: "song",
      label: () => "Queue one",
      icon: ListEnd,
      group: "queue",
      order: 10,
      run: (s) => single(s),
    },
    {
      id: "t.queue.many",
      target: "selection",
      kinds: ["song", "album"],
      label: () => "Queue all",
      icon: ListEnd,
      group: "queue",
      order: 10,
      run: (items, ctx) => queued(items, ctx),
    },
    {
      id: "t.love.many",
      target: "selection",
      kinds: ["song"],
      label: () => "Love all",
      icon: Star,
      group: "rating",
      order: 10,
      run: () => {},
    },
  ]);
});

afterEach(() => {
  cleanup();
  queued.mockClear();
  single.mockClear();
});

describe("bulk actions", () => {
  it("lists only the actions that take every selected kind", () => {
    expect(bulkActionsFor([song("a"), song("b")]).map((a) => a.id)).toEqual([
      "t.queue.many",
      "t.love.many",
    ]);
    expect(bulkActionsFor([song("a"), album("x")]).map((a) => a.id)).toEqual(["t.queue.many"]);
  });

  it("names a selection by kind and count", () => {
    expect(describeItems([song("a", "Airbag")])).toBe("“Airbag”");
    expect(describeItems([song("a"), song("b")])).toBe("2 songs");
    expect(describeItems([song("a"), album("x"), album("y")])).toBe("3 items");
  });
});

describe("a row's menu with a selection", () => {
  const rows = [song("a"), song("b"), song("c"), song("d")];
  const keys = rows.map((r) => r.target.id);

  function open(index: number, selectRows: number[]) {
    const scope = createSelectionScope({ keys, itemAt: (i) => rows[i] });
    let s = emptySelection;
    for (const i of selectRows) s = clickRow(s, keys, i, { range: false, toggle: true });
    scope.store.setState(s);
    render(
      <SelectionContext value={scope}>
        <ContextActionMenu
          item={rows[index]}
          name={rows[index].target.name}
          at={{ x: 10, y: 10 }}
          selectionKey={keys[index]}
          container={{ kind: "playlist", id: "p.1", editable: true, index }}
          onClose={() => {}}
        />
      </SelectionContext>,
      { wrapper: I18nRoot },
    );
  }

  it("runs the targets[] form on every selected row, in list order, with their positions", () => {
    open(3, [3, 0, 2]);
    expect(screen.getByText("3 songs selected")).toBeTruthy();
    fireEvent.click(screen.getByRole("menuitem", { name: "Queue all" }));
    expect(single).not.toHaveBeenCalled();
    expect(queued).toHaveBeenCalledTimes(1);
    const [items, ctx] = queued.mock.calls[0];
    expect(items.map((i) => i.target.id)).toEqual(["a", "c", "d"]);
    expect(ctx.container).toEqual({
      kind: "playlist",
      id: "p.1",
      editable: true,
      indices: [0, 2, 3],
    });
  });

  it("acts on the row alone when it is not part of a multi-selection", () => {
    open(1, [0, 2]);
    expect(screen.queryByText(/selected/)).toBeNull();
    fireEvent.click(screen.getByRole("menuitem", { name: "Queue one" }));
    expect(queued).not.toHaveBeenCalled();
    expect(single).toHaveBeenCalledWith(rows[1].target);
  });
});
