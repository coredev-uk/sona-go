import { t } from "@lingui/core/macro";

import { registerActions } from "@/shared/actions/registry";
import type { FeatureModule, NavItem, NavSection, ShellSlots, Shortcut } from "@/shared/module";
import { registerShortcuts } from "@/shared/shortcuts/registry";

import { modules } from "./modules";
import { shellShortcuts } from "./shortcuts";

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

/** Modifier for the positional "go to" keys of each section (Ctrl+1…9, Alt+1…9). */
// eslint-disable-next-line lingui/no-unlocalized-strings -- chord modifier identifiers, not text
const positional: Partial<Record<NavSection, string>> = { primary: "Ctrl", library: "Alt" };

/** A "go to" shortcut per sidebar link: `nav.<path>` ("nav.home", "nav.library.albums"). */
export function navShortcuts(sections: Record<NavSection, readonly NavItem[]>): Shortcut[] {
  return (Object.keys(sections) as NavSection[]).flatMap((section) =>
    sections[section].map((item, i): Shortcut => {
      const mod = positional[section];
      return {
        id: `nav.${item.to.split("/").filter(Boolean).join(".") || "home"}`,
        label:
          section === "library"
            ? () => {
                const page = item.label();
                return t`Library · ${page}`;
              }
            : item.label,
        group: "pages",
        keys: item.keys ?? (mod && i < 9 ? [`${mod}+${i + 1}`] : []),
        scope: "global",
        keywords: section === "library" ? () => t`library` : undefined,
        icon: item.icon,
        run: ({ navigate }) => void navigate({ to: item.to }),
      };
    }),
  );
}

/** Shell slots, in module order. */
export const slots: ShellSlots[] = modules.flatMap((m) => (m.shellSlots ? [m.shellSlots] : []));

/** Puts every module's item actions in the registry; call once before the first render. */
export function registerModuleActions(): void {
  for (const m of modules) if (m.actions) registerActions(m.actions);
}

/**
 * Puts every shortcut in the registry: the pages' "go to" keys, the
 * shell's, then each module's in module order. Call once before the first render.
 */
export function registerModuleShortcuts(): void {
  registerShortcuts([
    ...navShortcuts(nav),
    ...shellShortcuts,
    ...modules.flatMap((m) => m.shortcuts ?? []),
  ]);
}
