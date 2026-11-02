import { useLingui } from "@lingui/react/macro";
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";

import { AppearanceSection } from "./AppearanceSection";
import { ConnectionsSection } from "./Connections";
import { DiagnosticsSection } from "./Diagnostics";
import { focusSetting, sectionTitle } from "./find";
import { FindSetting } from "./FindSetting";
import { GeneralSection } from "./General";
import { PlaybackSection } from "./Playback";
import { settingsSections, type SettingsSection } from "./search-params";
import { ShortcutsSection } from "./Shortcuts";
import { StorageSection } from "./Storage";
import { useSectionSummaries } from "./summaries";

const route = getRouteApi("/settings");

/**
 * The section shown last in this run: /settings without a section (Ctrl+,,
 * the sidebar, the command palette) opens it again.
 */
let lastSection: SettingsSection = settingsSections[0];

function SectionBody({ section }: { section: SettingsSection }) {
  switch (section) {
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

/** The section list: each section's name over its state in one line (summaries.ts). */
function SectionList({ section }: { section: SettingsSection }) {
  const { t } = useLingui();
  const summaries = useSectionSummaries();
  return (
    <nav aria-label={t`Settings sections`}>
      <ul className="flex flex-col gap-0.5 max-[960px]:flex-row max-[960px]:flex-wrap">
        {settingsSections.map((s) => {
          const summary = summaries[s];
          return (
            <li key={s}>
              <Link
                to="/settings"
                search={{ section: s }}
                replace
                aria-current={s === section ? "page" : undefined}
                aria-labelledby={`settings-nav-${s}`}
                aria-describedby={summary ? `settings-nav-${s}-state` : undefined}
                className="group flex min-h-8 flex-col justify-center rounded-md px-3 py-1.5 transition-colors duration-100 hover:bg-hover aria-[current=page]:bg-active max-[960px]:px-2.5"
              >
                <span
                  id={`settings-nav-${s}`}
                  className="text-ui font-medium text-ink-muted group-hover:text-ink group-aria-[current=page]:text-ink"
                >
                  {sectionTitle(s)}
                </span>
                {summary && (
                  <span
                    id={`settings-nav-${s}-state`}
                    className="line-clamp-2 text-meta text-ink-muted max-[960px]:sr-only"
                  >
                    {summary}
                  </span>
                )}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}

/**
 * Settings is a page (a route), not a dialog: deep links name a section,
 * Back and Forward leave it like any page, the player bar stays usable
 * while you change what you hear, and the 940×600 window keeps its full
 * content area. Choosing a section replaces the history entry, so Back
 * leaves Settings instead of stepping through the sections.
 *
 * The left column is the page's index: its title, Find a setting and the
 * section list, held in view while a long section scrolls. Below 960px it
 * wraps above the section.
 */
export function SettingsPage() {
  const { t } = useLingui();
  const navigate = useNavigate();
  const section = route.useSearch().section ?? lastSection;
  useEffect(() => {
    lastSection = section;
  }, [section]);
  return (
    <div className="grid grid-cols-[240px_minmax(0,800px)] gap-x-12 max-[960px]:grid-cols-1 max-[960px]:gap-y-6">
      {/* Held in view only where the window is tall enough to show all of it. */}
      <div className="top-7 self-start min-[961px]:[@media(min-height:700px)]:sticky">
        <FindSetting
          title={<h1 className="text-page text-ink">{t`Settings`}</h1>}
          onPick={(e) => {
            if (e.section !== section) {
              void navigate({ to: "/settings", search: { section: e.section }, replace: true });
            }
            focusSetting(e.id);
          }}
        >
          <SectionList section={section} />
        </FindSetting>
      </div>
      <section aria-labelledby="settings-section-title" className="min-w-0">
        {/* Below 960px the wrapped section list above names the section already. */}
        <h2 id="settings-section-title" className="mb-6 text-page text-ink max-[960px]:sr-only">
          {sectionTitle(section)}
        </h2>
        <SectionBody section={section} />
      </section>
    </div>
  );
}
