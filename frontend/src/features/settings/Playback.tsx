import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";

import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { useCapabilities } from "@/shared/lib/capabilities";
import { numberFormat } from "@/shared/lib/format";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Slider } from "@/shared/ui/slider";

import { AudioRows } from "./AudioRows";
import { CdmRow } from "./CdmRow";
import { CrossfadeRows } from "./CrossfadeRows";
import { PrivateRows } from "./PrivateRows";
import { QueueRows } from "./QueueRows";
import { useSettings, useUpdateSettings } from "./queries";
import { SettingRow, SettingsPanel } from "./SettingRow";

const idleChoices = [5, 10, 15, 30, 60];

/**
 * Settings → Playback. Rows for what this platform's playback can't do are
 * left out; without any playback, one line says so at the top.
 */
export function PlaybackSection() {
  const { t } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  const caps = useCapabilities();
  if (settings.error) {
    return (
      <ErrorBlock
        title={t`Couldn't read Sona's settings`}
        error={settings.error}
        onRetry={() => void settings.refetch()}
      />
    );
  }
  const s = settings.data;
  if (!s) return <SettingsPanel className="h-72" />;
  const save = (patch: Partial<typeof s>) => update.mutate({ ...s, ...patch });
  const idle = idleChoices.includes(s.engineIdleMinutes)
    ? idleChoices
    : [...idleChoices, s.engineIdleMinutes].sort((a, b) => a - b);
  const volume = numberFormat({ style: "percent", maximumFractionDigits: 0 }).format(s.volume);
  const cdm = caps?.playback.engine === "cdm";

  return (
    <SettingsPanel>
      {caps && !caps.playback.available && (
        <div className="py-4">
          <p className="text-ui font-medium text-ink">
            <Trans>Playback isn't available on this platform yet</Trans>
          </p>
          <p className="mt-0.5 max-w-[56ch] text-sub text-ink-muted">
            <Trans>Browsing Apple Music and your library works as usual.</Trans>
          </p>
        </div>
      )}
      {cdm && <CdmRow />}
      <PrivateRows />
      <SettingRow
        id="quality"
        title={t`Audio quality`}
        description={t`Sona streams AAC: 256 kbps at High, 64 kbps with Data saver for slow or metered connections. Changes apply from the next song. Lossless and Dolby Atmos aren't available, so albums marked with them play as AAC too.`}
      >
        <Select
          value={String(s.audioQuality)}
          onValueChange={(v) => save({ audioQuality: Number(v) })}
        >
          <SelectTrigger id="quality" aria-describedby="quality-desc" className="h-8 w-44">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="256">
              <Trans>High · 256 kbps</Trans>
            </SelectItem>
            <SelectItem value="64">
              <Trans>Data saver · 64 kbps</Trans>
            </SelectItem>
          </SelectContent>
        </Select>
      </SettingRow>
      {caps?.playback.volume && (
        <SettingRow
          id="volume"
          title={t`Volume`}
          description={t`The same volume as the slider in the player bar. Its steps follow loudness, so the lower half stays useful.`}
        >
          <div
            className="flex w-44 items-center gap-3"
            style={{ "--slider-range": "var(--ink-muted)" } as React.CSSProperties}
          >
            <Slider
              id="volume"
              aria-labelledby="volume-label"
              aria-describedby="volume-desc"
              value={[Math.round(s.volume * 100)]}
              max={100}
              step={1}
              onValueCommit={([v]) => save({ volume: v / 100 })}
            />
            <span className="w-9 text-right text-meta text-ink-muted num">{volume}</span>
          </div>
        </SettingRow>
      )}
      {cdm && (
        <SettingRow
          id="idle"
          title={t`Stop the playback host when idle`}
          description={t`The playback host holds the Widevine CDM and the audio pipeline. With nothing queued, it shuts down after this long to free memory; the next song takes a few seconds longer to start. Applies the next time it starts.`}
        >
          <Select
            value={String(s.engineIdleMinutes)}
            onValueChange={(v) => save({ engineIdleMinutes: Number(v) })}
          >
            <SelectTrigger id="idle" aria-describedby="idle-desc" className="h-8 w-44">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {idle.map((m) => (
                <SelectItem key={m} value={String(m)}>
                  {t`After ${plural(m, { one: "# minute", other: "# minutes" })}`}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingRow>
      )}
      <AudioRows />
      <CrossfadeRows />
      <QueueRows />
    </SettingsPanel>
  );
}
