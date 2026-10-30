import type { Album, Artist, Playlist, Song, Station } from "@bindings/services";
import type { UseNavigateResult } from "@tanstack/react-router";
import { plural, t } from "@lingui/core/macro";
import type { LucideIcon } from "lucide-react";

/** What an item action can act on, by kind. */
export interface ActionTargets {
  song: Song;
  album: Album;
  playlist: Playlist;
  artist: Artist;
  station: Station;
}
export type ActionTargetKind = keyof ActionTargets;

/** One item of any kind: what a selection holds and a drag carries. */
export type ActionItem = {
  [K in ActionTargetKind]: { kind: K; target: ActionTargets[K] };
}[ActionTargetKind];

/** Menu sections in display order; a separator divides neighbouring non-empty sections. */
export const actionGroups = ["queue", "item", "rating"] as const;
export type ActionGroup = (typeof actionGroups)[number];

/**
 * Data an action reads only when its menu opens, not for every row (a
 * song's rating). Actions sharing a `MenuData` (by `id`) share one load.
 */
export interface MenuData<K extends ActionTargetKind, V> {
  id: string;
  /** Resolves the value; it should not reject (map failures to a fallback). */
  load(target: ActionTargets[K]): Promise<V>;
}

/**
 * The collection a menu's item is shown in, passed down by the list that
 * renders the menu (a playlist's track list), so actions can act on the
 * item's place in it ("Remove from this playlist").
 */
export interface ActionContainer {
  /** What the collection is: "playlist", "album", … */
  kind: string;
  /** The collection's ID. */
  id: string;
  /** The item's position in the collection as listed. */
  index: number;
  /** The user may change the collection's items. */
  editable: boolean;
}

export interface ActionContext<K extends ActionTargetKind> {
  navigate: UseNavigateResult<string>;
  /** The value `data` loaded when the menu opened; `undefined` until it resolves. */
  data<V>(data: MenuData<K, V>): V | undefined;
  /** The collection the item is shown in, when its list passes one. */
  container?: ActionContainer;
}

type Read<K extends ActionTargetKind, R> = (target: ActionTargets[K], ctx: ActionContext<K>) => R;

export interface ActionDef<K extends ActionTargetKind> {
  /** Unique across the registry, e.g. "player.play-next". */
  id: string;
  target: K;
  /** Translated when the menu renders: (target) => t`Play next`. */
  label: Read<K, string>;
  icon: LucideIcon;
  /** Draws the icon filled (a loved heart); icons are outlined otherwise. */
  filled?: Read<K, boolean>;
  group: ActionGroup;
  /** Position inside the group, ascending. */
  order: number;
  /** Whether the action is listed for this target at all (default: always). */
  isAvailable?: Read<K, boolean>;
  /** Listed but greyed out when false (default: enabled). */
  isEnabled?: Read<K, boolean>;
  /** Menu data the action reads through `ctx.data`, loaded when the menu opens. */
  data?: readonly MenuData<K, unknown>[];
  run: Read<K, void>;
}

/** Any action, whatever its target kind. */
export type AnyActionDef = { [K in ActionTargetKind]: ActionDef<K> }[ActionTargetKind];

/** Data a bulk action reads when its menu opens (the selection's ratings), shared by `id`. */
export interface BulkMenuData<V> {
  id: string;
  /** Resolves the value; it should not reject (map failures to a fallback). */
  load(items: readonly ActionItem[]): Promise<V>;
}

export interface BulkActionContext {
  navigate: UseNavigateResult<string>;
  /** The value `data` loaded when the menu opened; `undefined` until it resolves. */
  data<V>(data: BulkMenuData<V>): V | undefined;
  /** The collection the items are listed in, with each item's position in it. */
  container?: Omit<ActionContainer, "index"> & { indices: readonly number[] };
}

type ReadMany<R> = (items: readonly ActionItem[], ctx: BulkActionContext) => R;

/**
 * The `targets[]` form of an action: one run receives every selected item,
 * in list order. It is listed only when every selected item is one of its
 * `kinds`, so a mixed selection (albums and playlists) shows the actions
 * they share.
 */
export interface BulkActionDef {
  /** Unique across the registry, e.g. "player.play-next.many". */
  id: string;
  /** Tells the targets[] form from the single-item ones. */
  target: "selection";
  kinds: readonly ActionTargetKind[];
  label: ReadMany<string>;
  icon: LucideIcon;
  filled?: ReadMany<boolean>;
  group: ActionGroup;
  order: number;
  isAvailable?: ReadMany<boolean>;
  isEnabled?: ReadMany<boolean>;
  data?: readonly BulkMenuData<unknown>[];
  run: ReadMany<void>;
}

/** What a module registers: single-item actions and their `targets[]` forms. */
export type ItemActionDef = AnyActionDef | BulkActionDef;

const registry: { [K in ActionTargetKind]: ActionDef<K>[] } = {
  song: [],
  album: [],
  playlist: [],
  artist: [],
  station: [],
};
const bulk: BulkActionDef[] = [];
const ids = new Set<string>();

const rank = (a: { group: ActionGroup; order: number }) =>
  actionGroups.indexOf(a.group) * 1000 + a.order;

/** Adds actions to the registry; menus list them by group, then order. */
export function registerActions(defs: readonly ItemActionDef[]): void {
  for (const def of defs) {
    if (ids.has(def.id)) throw new Error(`action ${def.id} registered twice`);
    ids.add(def.id);
    const list: { group: ActionGroup; order: number }[] =
      def.target === "selection" ? bulk : (registry[def.target] as AnyActionDef[]);
    list.push(def);
    list.sort((a, b) => rank(a) - rank(b));
  }
}

/** Every registered action for a kind of target, in menu order. */
export function actionsFor<K extends ActionTargetKind>(kind: K): readonly ActionDef<K>[] {
  return registry[kind];
}

/** The bulk actions that accept every item of `items`, in menu order. */
export function bulkActionsFor(items: readonly ActionItem[]): readonly BulkActionDef[] {
  const kinds = new Set(items.map((i) => i.kind));
  return bulk.filter((a) => [...kinds].every((k) => a.kinds.includes(k)));
}

function countOf(kind: ActionTargetKind, n: number): string {
  switch (kind) {
    case "song":
      return plural(n, { one: "# song", other: "# songs" });
    case "album":
      return plural(n, { one: "# album", other: "# albums" });
    case "playlist":
      return plural(n, { one: "# playlist", other: "# playlists" });
    case "artist":
      return plural(n, { one: "# artist", other: "# artists" });
    case "station":
      return plural(n, { one: "# station", other: "# stations" });
  }
}

/** What items are, for menus and toasts: “Airbag”, "12 songs", "3 albums", "5 items". */
export function describeItems(items: readonly ActionItem[]): string {
  if (items.length === 1) {
    const name = items[0].target.name;
    return t`“${name}”`;
  }
  const kinds = new Set(items.map((i) => i.kind));
  if (kinds.size === 1) return countOf(items[0].kind, items.length);
  return plural(items.length, { one: "# item", other: "# items" });
}

/** Splits ordered actions into their non-empty groups, in group order. */
export function menuSections<A extends { group: ActionGroup }>(
  actions: readonly A[],
): { group: ActionGroup; actions: A[] }[] {
  return actionGroups
    .map((group) => ({ group, actions: actions.filter((a) => a.group === group) }))
    .filter((s) => s.actions.length > 0);
}
