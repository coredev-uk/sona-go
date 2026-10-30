import type { Album, Artist, Playlist, Song, Station } from "@bindings/services";
import type { UseNavigateResult } from "@tanstack/react-router";
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
  label: string | Read<K, string>;
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

const registry: { [K in ActionTargetKind]: ActionDef<K>[] } = {
  song: [],
  album: [],
  playlist: [],
  artist: [],
  station: [],
};

const rank = (a: AnyActionDef) => actionGroups.indexOf(a.group) * 1000 + a.order;

/** Adds actions to the registry; menus list them by group, then order. */
export function registerActions(defs: readonly AnyActionDef[]): void {
  for (const def of defs) {
    const list = registry[def.target] as AnyActionDef[];
    if (list.some((a) => a.id === def.id)) throw new Error(`action ${def.id} registered twice`);
    list.push(def);
    list.sort((a, b) => rank(a) - rank(b));
  }
}

/** Every registered action for a kind of target, in menu order. */
export function actionsFor<K extends ActionTargetKind>(kind: K): readonly ActionDef<K>[] {
  return registry[kind];
}

/** Splits ordered actions into their non-empty groups, in group order. */
export function menuSections<A extends { group: ActionGroup }>(
  actions: readonly A[],
): { group: ActionGroup; actions: A[] }[] {
  return actionGroups
    .map((group) => ({ group, actions: actions.filter((a) => a.group === group) }))
    .filter((s) => s.actions.length > 0);
}
