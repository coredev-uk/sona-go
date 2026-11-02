import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";

import { nav, slots, type ModuleNavItem } from "../compose";
import { TitleBarStart } from "./TitleBarStart";
import { useWindowChrome } from "./window-chrome";

const footers = slots.flatMap((s) => (s.SidebarFooter ? [s.SidebarFooter] : []));
const sections = slots.flatMap((s) => (s.SidebarSection ? [s.SidebarSection] : []));

function NavLink({ item }: { item: ModuleNavItem }) {
  const Icon = item.icon;
  return (
    <li>
      <Link
        to={item.to}
        activeOptions={{ exact: item.exact, includeSearch: false }}
        className="flex h-8 items-center gap-2.5 rounded-md px-2.5 text-ui text-ink-muted transition-colors duration-100 hover:bg-hover hover:text-ink data-[status=active]:bg-active data-[status=active]:font-medium data-[status=active]:text-ink"
      >
        <Icon aria-hidden className="size-4 shrink-0" strokeWidth={1.75} />
        <span className="truncate">{item.label()}</span>
      </Link>
    </li>
  );
}

/**
 * The sidebar: modules' nav items by section, module sidebar sections
 * (pins, playlists) in the scrolling middle, and module footer slots
 * (sync status).
 */
export function Sidebar() {
  const { t } = useLingui();
  const chrome = useWindowChrome();
  return (
    <nav aria-label={t`Main`} className="flex min-h-0 flex-col bg-surface px-3 pb-3">
      {/* The title bar's leading end while the sidebar is shown. */}
      <TitleBarStart chrome={chrome} className="-mx-3 pe-[22px]" />
      <ul className="mt-2 flex flex-col gap-0.5">
        {nav.primary.map((item) => (
          <NavLink key={item.to} item={item} />
        ))}
      </ul>
      <div className="-mx-3 min-h-0 flex-1 overflow-y-auto px-3 pb-4">
        {nav.library.length > 0 && (
          <>
            <h2 className="mt-6 mb-1.5 px-2.5 text-meta text-ink-faint">
              <Trans>Library</Trans>
            </h2>
            <ul className="flex flex-col gap-0.5">
              {nav.library.map((item) => (
                <NavLink key={item.to} item={item} />
              ))}
            </ul>
          </>
        )}
        {sections.map((Section, i) => (
          <Section key={i} />
        ))}
      </div>
      <div className="shrink-0">
        {footers.map((Footer, i) => (
          <Footer key={i} />
        ))}
        <ul className="flex flex-col gap-0.5">
          {nav.footer.map((item) => (
            <NavLink key={item.to} item={item} />
          ))}
        </ul>
      </div>
    </nav>
  );
}
