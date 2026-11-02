import { useLingui } from "@lingui/react/macro";

import { useSettings, useUpdateSettings, type Theme } from "@/features/settings";
import { cn } from "@/shared/lib/utils";

/**
 * Sona's window in miniature, in one palette (`.theme-light` or `.dark`
 * scope the tokens to it whatever the page's theme): the sidebar with the
 * mark, the top bar, a few rows, and the player bar with its amber play
 * control. Drawn from the tokens, so it follows any change to them.
 */
function Window({ palette }: { palette: "theme-light" | "dark" }) {
  return (
    <div className={cn(palette, "absolute inset-0 bg-ground")}>
      <div className="absolute inset-y-0 start-0 w-[28%] bg-surface">
        <div className="absolute start-[14%] top-[8%] aspect-square w-[22%] rounded-full bg-play" />
        <div className="absolute start-[14%] top-[30%] h-[5%] w-[58%] rounded-full bg-ink-faint/40" />
        <div className="absolute start-[14%] top-[42%] h-[5%] w-[44%] rounded-full bg-ink-faint/40" />
        <div className="absolute start-[14%] top-[54%] h-[5%] w-[52%] rounded-full bg-ink-faint/40" />
      </div>
      <div className="absolute end-0 top-0 h-[16%] w-[72%] border-b border-line bg-surface" />
      <div className="absolute start-[34%] top-[26%] h-[8%] w-[30%] rounded-full bg-ink/80" />
      <div className="absolute start-[34%] top-[42%] h-[5%] w-[50%] rounded-full bg-ink-muted/40" />
      <div className="absolute start-[34%] top-[52%] h-[5%] w-[42%] rounded-full bg-ink-muted/40" />
      <div className="absolute start-[34%] top-[62%] h-[5%] w-[46%] rounded-full bg-ink-muted/40" />
      <div className="absolute inset-x-0 bottom-0 h-[18%] border-t border-line bg-surface">
        <div className="absolute top-1/2 left-1/2 aspect-square h-[48%] -translate-1/2 rounded-full bg-play" />
      </div>
    </div>
  );
}

/** The light and dark windows, or both halved for System. */
function Preview({ theme }: { theme: Theme }) {
  return (
    <div aria-hidden className="relative aspect-[16/10] w-full overflow-hidden rounded-md">
      {theme === "system" ? (
        <>
          <Window palette="theme-light" />
          <div className="absolute inset-0 [clip-path:inset(0_0_0_50%)]">
            <Window palette="dark" />
          </div>
        </>
      ) : (
        <Window palette={theme === "dark" ? "dark" : "theme-light"} />
      )}
      {/* The edge, over the picture so a dark window still shows on a dark card. */}
      <div className="absolute inset-0 rounded-md ring-1 ring-ink/20 ring-inset" />
    </div>
  );
}

/**
 * The theme as three pictures to choose from (Settings → Appearance has
 * the same setting as a list). Radio buttons: Tab reaches the chosen one,
 * the arrow keys choose, and the whole setup changes at once.
 */
export function ThemePicker() {
  const { t } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  const saved = settings.data?.theme;
  const theme: Theme = saved === "light" || saved === "dark" ? saved : "system";
  const choices: { value: Theme; label: string; hint: string }[] = [
    { value: "system", label: t`System`, hint: t`Follows your desktop` },
    { value: "light", label: t`Light`, hint: t`Always light` },
    { value: "dark", label: t`Dark`, hint: t`Always dark` },
  ];
  return (
    <section aria-labelledby="setup-theme" className="flex flex-col gap-2">
      <h2 id="setup-theme" className="text-ui font-semibold text-ink">
        {t`Theme`}
      </h2>
      <div
        role="radiogroup"
        aria-labelledby="setup-theme"
        className="grid grid-cols-3 gap-3 max-[1080px]:gap-2"
      >
        {choices.map((c) => (
          <label key={c.value} className="group relative flex cursor-pointer flex-col">
            <input
              type="radio"
              name="setup-theme"
              value={c.value}
              checked={theme === c.value}
              disabled={!settings.data}
              onChange={() => {
                if (settings.data) update.mutate({ ...settings.data, theme: c.value });
              }}
              className="peer sr-only"
            />
            <span className="flex flex-col gap-3 rounded-lg bg-surface p-3 transition-colors duration-100 group-hover:bg-hover peer-checked:ring-2 peer-checked:ring-ink peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-ring">
              <Preview theme={c.value} />
              <span className="flex flex-col">
                <span className="text-ui font-medium text-ink">{c.label}</span>
                <span className="text-sub text-ink-muted">{c.hint}</span>
              </span>
            </span>
          </label>
        ))}
      </div>
    </section>
  );
}
