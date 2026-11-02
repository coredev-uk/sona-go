import type { Settings } from "@bindings/config";
import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { useState, type CSSProperties } from "react";

import { isForced, setPrivate, usePrivate } from "@/features/private";
import { useSettings, useUpdateSettings } from "@/features/settings";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { useCapabilities } from "@/shared/lib/capabilities";
import { Slider } from "@/shared/ui/slider";

import { ChoiceCards, Loading, StepGroup, SwitchList, SwitchRow } from "./parts";
import { WidevineStatus } from "./WidevineStatus";

/** Crossfade's length under its switch while it is on; saved when the thumb is let go. */
function CrossfadeLength({ s }: { s: Settings }) {
  const { t } = useLingui();
  const update = useUpdateSettings();
  const [dragging, setDragging] = useState<number | null>(null);
  const seconds = dragging ?? s.crossfade.seconds;
  return (
    <div
      className="mt-2 flex items-center gap-3"
      style={{ "--slider-range": "var(--ink-muted)" } as CSSProperties}
    >
      <span id="setup-crossfade-length" className="text-sub text-ink-muted">
        <Trans>Length</Trans>
      </span>
      <Slider
        aria-labelledby="setup-crossfade-length"
        aria-valuetext={t`${plural(seconds, { one: "# second", other: "# seconds" })}`}
        value={[seconds]}
        min={1}
        max={12}
        step={1}
        className="max-w-56"
        onValueChange={([v]) => setDragging(v)}
        onValueCommit={([v]) => {
          setDragging(null);
          update.mutate({ ...s, crossfade: { ...s.crossfade, seconds: v } });
        }}
      />
      <span aria-hidden className="w-9 text-meta text-ink-muted num">
        <Trans>{seconds} s</Trans>
      </span>
    </div>
  );
}

/**
 * Playback: Widevine first, then the sound (quality as two cards, Sound
 * Check, crossfade) and private listening, each only where the platform's
 * playback can do it. Settings → Playback has these and more.
 */
export function PlaybackStep() {
  const { t } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  const playback = useCapabilities()?.playback;
  const priv = usePrivate((p) => p.state);
  const s = settings.data;

  let body = null;
  if (settings.error) {
    body = (
      <ErrorBlock
        title={t`Couldn't read Sona's settings`}
        error={settings.error}
        onRetry={() => void settings.refetch()}
        retrying={settings.isRefetching}
      />
    );
  } else if (!s || !playback) {
    body = <Loading className="h-64" />;
  } else if (playback.available) {
    const forced = isForced(priv);
    const privateHint = forced
      ? t`On because Sona was started with SONA_PRIVATE=1.`
      : priv?.source === "development"
        ? t`On in development builds; switching it here lasts until Sona quits.`
        : t`Plays stay out of your Apple Music history and recommendations, and aren't scrobbled.`;
    body = (
      <>
        <StepGroup id="setup-quality" title={t`Audio quality`}>
          <ChoiceCards
            name="setup-quality"
            labelledBy="setup-quality"
            value={String(s.audioQuality)}
            choices={[
              { value: "256", label: t`High quality`, hint: t`AAC at 256 kbps` },
              {
                value: "64",
                label: t`Data saver`,
                hint: t`AAC at 64 kbps, for metered connections`,
              },
            ]}
            onChange={(v) => update.mutate({ ...s, audioQuality: Number(v) })}
          />
        </StepGroup>
        <SwitchList>
          {playback.soundCheck && (
            <SwitchRow
              id="setup-sound-check"
              label={t`Sound Check`}
              hint={t`Plays every song at about the same loudness.`}
              checked={s.audio.soundCheck}
              onChange={(soundCheck) => update.mutate({ ...s, audio: { ...s.audio, soundCheck } })}
            />
          )}
          {playback.crossfade && (
            <SwitchRow
              id="setup-crossfade"
              label={t`Crossfade`}
              hint={t`Fades each song into the next. Songs of one album still play without a gap.`}
              checked={s.crossfade.enabled}
              onChange={(enabled) =>
                update.mutate({ ...s, crossfade: { ...s.crossfade, enabled } })
              }
            >
              {s.crossfade.enabled && <CrossfadeLength s={s} />}
            </SwitchRow>
          )}
          {playback.privateListening && (
            <SwitchRow
              id="setup-private"
              label={t`Private listening`}
              hint={privateHint}
              checked={priv?.on ?? false}
              disabled={!priv || forced}
              onChange={(on) => void setPrivate(on)}
            />
          )}
        </SwitchList>
      </>
    );
  }
  return (
    <div className="flex flex-col gap-5">
      <WidevineStatus />
      {body}
    </div>
  );
}
