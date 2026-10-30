import type { Equalizer } from "@bindings/config";
import type { EqualizerPreset } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { useState } from "react";

import { useSettings, useUpdateSettings } from "@/features/settings";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { PageHeader } from "@/shared/components/PageHeader";
import { cn } from "@/shared/lib/utils";
import { numberFormat } from "@/shared/lib/format";
import { Switch } from "@/shared/ui/switch";

import { CUSTOM, formatDb, formatHz, gainsOf, headroom, presetName, withBand } from "./curve";
import { Fader } from "./Fader";
import { preview, useEqualizerInfo } from "./queries";
import { ResponseGraph } from "./ResponseGraph";

/** The preset list: the user's own curve first, then the built-in curves. */
function Presets({
  presets,
  value,
  onSelect,
}: {
  presets: readonly EqualizerPreset[];
  value: string;
  onSelect: (id: string) => void;
}) {
  const { t } = useLingui();
  const items = [{ id: CUSTOM, name: "" }, ...presets];
  const move = (from: number, by: number) => {
    const next = items[(from + by + items.length) % items.length];
    onSelect(next.id);
    document.getElementById(`eq-preset-${next.id}`)?.focus();
  };
  return (
    <div role="radiogroup" aria-label={t`Preset`} className="flex flex-col gap-0.5">
      {items.map((p, i) => {
        const on = p.id === value;
        return (
          <button
            key={p.id}
            id={`eq-preset-${p.id}`}
            type="button"
            role="radio"
            aria-checked={on}
            tabIndex={on ? 0 : -1}
            onClick={() => onSelect(p.id)}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown" || e.key === "ArrowRight") move(i, 1);
              else if (e.key === "ArrowUp" || e.key === "ArrowLeft") move(i, -1);
              else return;
              e.preventDefault();
            }}
            className={cn(
              "flex h-8 items-center rounded-md px-2.5 text-left text-ui transition-colors duration-100",
              on
                ? "bg-active font-medium text-ink"
                : "text-ink-muted hover:bg-hover hover:text-ink",
            )}
          >
            {presetName(p)}
          </button>
        );
      })}
    </div>
  );
}

/**
 * The equaliser: ten bands drawn as one response curve over their faders,
 * a preamp, and the presets. Moving a fader is heard at once (a preview
 * sent to the playback host) and saved when it is let go.
 */
export function EqualizerPage() {
  const { t } = useLingui();
  const settings = useSettings();
  const info = useEqualizerInfo();
  const update = useUpdateSettings();
  // The curve while a fader is held: shown and heard, saved on release.
  const [draft, setDraft] = useState<Equalizer | null>(null);

  const error = settings.error ?? info.error;
  if (error) {
    return (
      <>
        <PageHeader title={t`Equaliser`} />
        <ErrorBlock
          title={t`Couldn't load the equaliser`}
          error={error}
          onRetry={() => void Promise.all([settings.refetch(), info.refetch()])}
        />
      </>
    );
  }
  const s = settings.data;
  const eqInfo = info.data;
  if (!s || !eqInfo) {
    return (
      <>
        <PageHeader title={t`Equaliser`} />
        <div className="h-80 rounded-lg bg-surface" />
      </>
    );
  }

  const saved = s.audio.equalizer;
  const eq = draft ?? saved;
  const gains = gainsOf(eq, eqInfo.presets);
  const preset = eqInfo.presets.find((p) => p.id === eq.preset);
  const save = (next: Equalizer) => {
    setDraft(null);
    update.mutate({ ...s, audio: { ...s.audio, equalizer: next } });
  };
  const live = (next: Equalizer) => {
    setDraft(next);
    preview(next);
  };
  const room = -headroom(gains);
  const curveName = presetName(preset ?? { id: CUSTOM, name: "" });
  const drop = numberFormat({ maximumFractionDigits: 1 }).format(room);

  return (
    <>
      <PageHeader
        title={t`Equaliser`}
        subline={
          eq.enabled
            ? t`${curveName} · shapes everything Sona plays`
            : t`Off · Sona plays the music as mastered`
        }
        actions={
          <label className="flex items-center gap-2.5 text-ui text-ink">
            <Switch checked={eq.enabled} onCheckedChange={(enabled) => save({ ...eq, enabled })} />
            {eq.enabled ? <Trans>On</Trans> : <Trans>Off</Trans>}
          </label>
        }
      />
      <div className="grid max-w-[1040px] grid-cols-[176px_minmax(0,1fr)] gap-8 max-[960px]:grid-cols-1">
        <Presets
          presets={eqInfo.presets}
          value={eq.preset}
          onSelect={(id) => save({ ...eq, preset: id, enabled: true })}
        />
        <section aria-label={t`Bands`} className="min-w-0 rounded-lg bg-surface px-5 pt-5 pb-4">
          <div className="grid grid-cols-[56px_minmax(0,1fr)] gap-x-4">
            <div
              className="flex flex-col justify-between py-1 text-right text-meta text-ink-faint num"
              aria-hidden
            >
              <span>{formatDb(eqInfo.maxGain)}</span>
              <span>
                <Trans>0 dB</Trans>
              </span>
              <span>{formatDb(eqInfo.minGain)}</span>
            </div>
            <ResponseGraph
              freqs={eqInfo.frequencies}
              gains={gains}
              range={eqInfo.maxGain}
              dim={!eq.enabled}
            />
            <div className="mt-5">
              <Fader
                label={t`Preamp`}
                name={t`Preamp`}
                value={eq.preamp}
                min={eqInfo.minPreamp}
                max={eqInfo.maxPreamp}
                dim={!eq.enabled}
                onChange={(preamp) => live({ ...eq, preamp })}
                onCommit={(preamp) => save({ ...eq, preamp })}
              />
            </div>
            <div className="mt-5 grid grid-cols-10">
              {eqInfo.frequencies.map((hz, i) => {
                const freq = formatHz(hz);
                return (
                  <Fader
                    key={hz}
                    label={t`${freq} Hz`}
                    name={freq}
                    value={gains[i] ?? 0}
                    min={eqInfo.minGain}
                    max={eqInfo.maxGain}
                    dim={!eq.enabled}
                    onChange={(db) => live(withBand(eq, eqInfo.presets, i, db))}
                    onCommit={(db) => save(withBand(eq, eqInfo.presets, i, db))}
                  />
                );
              })}
            </div>
          </div>
          <p className="mt-4 text-meta text-ink-muted">
            {room > 0 ? (
              <Trans>
                The level drops {drop} dB ahead of the bands, so the largest boost cannot clip.
                Preamp lowers it further.
              </Trans>
            ) : (
              <Trans>Cuts only: the level stays as it is. Preamp lowers it further.</Trans>
            )}
          </p>
        </section>
      </div>
    </>
  );
}
