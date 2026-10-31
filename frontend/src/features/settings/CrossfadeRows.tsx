import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { useState } from "react";

import { useCapabilities } from "@/shared/lib/capabilities";
import { Slider } from "@/shared/ui/slider";
import { Switch } from "@/shared/ui/switch";

import { useSettings, useUpdateSettings } from "./queries";
import { SettingRow } from "./SettingRow";

/** Settings → Playback: crossfade between songs, and its length (where playback supports it). */
export function CrossfadeRows() {
  const { t } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  const supported = useCapabilities()?.playback.crossfade ?? false;
  const s = settings.data;
  // The length while the thumb moves; saved on release.
  const [dragging, setDragging] = useState<number | null>(null);
  if (!s || !supported) return null;
  const cf = s.crossfade;
  const seconds = dragging ?? cf.seconds;
  return (
    <>
      <SettingRow
        id="crossfade"
        title={t`Crossfade`}
        description={t`Fades each song into the next instead of playing them back to back. Songs that follow each other on the same album always play without a gap, as they were mastered.`}
      >
        <Switch
          id="crossfade"
          aria-describedby="crossfade-desc"
          checked={cf.enabled}
          onCheckedChange={(enabled) => update.mutate({ ...s, crossfade: { ...cf, enabled } })}
        />
      </SettingRow>
      {cf.enabled && (
        <SettingRow
          id="crossfade-seconds"
          title={t`Crossfade length`}
          description={t`How long the end of one song overlaps the start of the next. Applies from the next song.`}
        >
          <div
            className="flex w-44 items-center gap-3"
            style={{ "--slider-range": "var(--ink-muted)" } as React.CSSProperties}
          >
            <Slider
              id="crossfade-seconds"
              aria-labelledby="crossfade-seconds-label"
              aria-describedby="crossfade-seconds-desc"
              aria-valuetext={t`${plural(seconds, { one: "# second", other: "# seconds" })}`}
              value={[seconds]}
              min={1}
              max={12}
              step={1}
              onValueChange={([v]) => setDragging(v)}
              onValueCommit={([v]) => {
                setDragging(null);
                update.mutate({ ...s, crossfade: { ...cf, seconds: v } });
              }}
            />
            <span className="w-9 text-right text-meta text-ink-muted num">
              <Trans>{seconds} s</Trans>
            </span>
          </div>
        </SettingRow>
      )}
    </>
  );
}
