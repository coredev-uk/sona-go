import { getRouteApi, Link } from "@tanstack/react-router";

import { AccountSection } from "@/features/auth";
import { PageHeader } from "@/shared/components/PageHeader";

import { AppearanceSection } from "./Appearance";
import { DiagnosticsSection } from "./Diagnostics";
import { PlaybackSection } from "./Playback";
import { settingsSections, type SettingsSection } from "./search-params";

const route = getRouteApi("/settings");

const titles: Record<SettingsSection, string> = {
  account: "Account",
  playback: "Playback",
  appearance: "Appearance",
  diagnostics: "Diagnostics",
};

function SectionBody({ section }: { section: SettingsSection }) {
  switch (section) {
    case "account":
      return <AccountSection />;
    case "playback":
      return <PlaybackSection />;
    case "appearance":
      return <AppearanceSection />;
    case "diagnostics":
      return <DiagnosticsSection />;
  }
}

export function SettingsPage() {
  const { section } = route.useSearch();
  return (
    <>
      <PageHeader title="Settings" />
      <div className="grid grid-cols-[180px_minmax(0,720px)] gap-10 max-[960px]:grid-cols-1 max-[960px]:gap-6">
        <nav aria-label="Settings sections">
          <ul className="flex flex-col gap-0.5 max-[960px]:flex-row">
            {settingsSections.map((s) => (
              <li key={s}>
                <Link
                  to="/settings"
                  search={{ section: s }}
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
          <h2 id="settings-section-title" className="mb-3 text-heading text-ink">
            {titles[section]}
          </h2>
          <SectionBody section={section} />
        </section>
      </div>
    </>
  );
}
