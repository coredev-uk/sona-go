import { registerActions } from "@/shared/actions/registry";
import type { FeatureModule, NavItem, NavSection, ShellSlots } from "@/shared/module";

import { modules } from "./modules";

type RouteOf<M> = M extends FeatureModule<infer R> ? R : never;

/** The union of every registered module's routes. */
export type ModuleRoute = RouteOf<(typeof modules)[number]>;
export type ModuleNavItem = NavItem<ModuleRoute["fullPath"]>;

/** Every page route, in module order, for the root route's children. */
export const routes = modules.flatMap((m): readonly ModuleRoute[] => m.routes);

const navItems = modules.flatMap((m): readonly ModuleNavItem[] => m.nav ?? []);

/** Sidebar links by section, each sorted by `order`. */
export const nav: Record<NavSection, ModuleNavItem[]> = {
  primary: [],
  library: [],
  footer: [],
};
for (const item of navItems) nav[item.section].push(item);
for (const list of Object.values(nav)) list.sort((a, b) => a.order - b.order);

export const shortcuts = modules.flatMap((m) => m.shortcuts ?? []);

/** Shell slots, in module order. */
export const slots: ShellSlots[] = modules.flatMap((m) => (m.shellSlots ? [m.shellSlots] : []));

/** Puts every module's item actions in the registry; call once before the first render. */
export function registerModuleActions(): void {
  for (const m of modules) if (m.actions) registerActions(m.actions);
}
