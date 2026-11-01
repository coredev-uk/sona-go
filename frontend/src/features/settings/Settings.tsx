import { useLingui } from "@lingui/react/macro";
import { getRouteApi, Link } from "@tanstack/react-router";
import { useEffect } from "react";

import { AccountSection } from "@/features/auth";
import { PageHeader } from "@/shared/components/PageHeader";

import { AppearanceSection } from "./AppearanceSection";
import { ConnectionsSection } from "./Connections";
import { DiagnosticsSection } from "./Diagnostics";
import { GeneralSection } from "./General";
import { PlaybackSection } from "./Playback";
import { settingsSections, type SettingsSection } from "./search-params";
import { ShortcutsSection } from "./Shortcuts";
import { StorageSection } from "./Storage";

const route = getRouteApi("/settings");

/**
 * The section shown last in this run: /settings without a section (Ctrl+,,
 * the sidebar, the command palette) opens it again.
 */
let lastSection: SettingsSection = settingsSections[0];

function SectionBody({ section }: { section: SettingsSection }) {
  switch (section) {
    case "account":
      return <AccountSection />;
    case "general":
      return <GeneralSection />;
    case "playback":
      return <PlaybackSection />;
    case "appearance":
      return <AppearanceSection />;
    case "connections":
      return <ConnectionsSection />;
    case "shortcuts":
      return <ShortcutsSection />;
    case "storage":
      return <StorageSection />;
    case "diagnostics":
      return <DiagnosticsSection />;
  }
}

/**
 * Settings is a page (a route), not a dialog: deep links name a section,
 * Back and Forward leave it like any page, the player bar stays usable
 * while you change what you hear, and the 940×600 window keeps its full
 * content area. Choosing a section replaces the history entry, so Back
 * leaves Settings instead of stepping through the sections.
 */
export function SettingsPage() {
  const { t } = useLingui();
  const section = route.useSearch().section ?? lastSection;
  useEffect(() => {
    lastSection = section;
  }, [section]);
  const titles: Record<SettingsSection, string> = {
    account: t`Account`,
    general: t`General`,
    playback: t`Playback`,
    appearance: t`Appearance`,
    connections: t`Connections`,
    shortcuts: t`Shortcuts`,
    storage: t`Storage`,
    diagnostics: t`Diagnostics`,
  };
  return (
    <>
      <PageHeader title={t`Settings`} />
      <div className="grid grid-cols-[180px_minmax(0,720px)] gap-10 max-[960px]:grid-cols-1 max-[960px]:gap-6">
        <nav aria-label={t`Settings sections`}>
          <ul className="flex flex-col gap-0.5 max-[960px]:flex-row max-[960px]:flex-wrap">
            {settingsSections.map((s) => (
              <li key={s}>
                <Link
                  to="/settings"
                  search={{ section: s }}
                  replace
                  aria-current={s === section ? "page" : undefined}
                  className="flex h-8 items-center rounded-md px-2.5 text-ui text-ink-muted transition-colors duration-100 hover:bg-hover hover:text-ink aria-[current=page]:bg-active aria-[current=page]:font-medium aria-[current=page]:text-ink"
                >
                  {titles[s]}
                </Link>
              </li>
            ))}
          </ul>
        </nav>
        <section aria-labelledby="settings-section-title" className="min-w-0">
          {/* Below 960px the wrapped section list above names the section already. */}
          <h2
            id="settings-section-title"
            className="mb-4 text-heading text-ink max-[960px]:sr-only"
          >
            {titles[section]}
          </h2>
          <SectionBody section={section} />
        </section>
      </div>
    </>
  );
}
