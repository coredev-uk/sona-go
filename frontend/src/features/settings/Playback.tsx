import { DesktopService } from "@bindings/services";
import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { useState } from "react";
import { toast } from "sonner";

import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { callFailure } from "@/shared/lib/errors";
import { numberFormat } from "@/shared/lib/format";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Slider } from "@/shared/ui/slider";

import { AudioRows } from "./AudioRows";
import { CrossfadeRows } from "./CrossfadeRows";
import { QueueRows } from "./QueueRows";
import { useSettings, useUpdateSettings } from "./queries";
import { SettingRow, SettingsPanel } from "./SettingRow";

const idleChoices = [5, 10, 15, 30, 60];

/**
 * The CDM location: the chosen path (or "found automatically"), a button
 * that opens the desktop's file chooser through Go (xdg-desktop-portal),
 * and a way back to the automatic search.
 */
function CdmPathField({ value, onSave }: { value: string; onSave: (v: string) => void }) {
  const { t } = useLingui();
  const [choosing, setChoosing] = useState(false);
  const choose = async () => {
    setChoosing(true);
    try {
      const path = await DesktopService.ChooseCDMPath();
      // "" when the dialog was cancelled.
      if (path && path !== value) onSave(path);
    } catch (err) {
      toast.error(t`Couldn't open the file chooser`, { description: callFailure(err).message });
    }
    setChoosing(false);
  };
  return (
    <div className="flex gap-2">
      <output
        id="cdm-path"
        aria-describedby="cdm-path-desc"
        title={value || undefined}
        className={cn(
          "flex h-8 min-w-0 flex-1 items-center truncate rounded-md border border-control-line bg-raised px-3 text-ui",
          value ? "text-ink" : "text-ink-faint",
        )}
      >
        <span className="truncate">{value || <Trans>Found automatically</Trans>}</span>
      </output>
      <Button variant="outline" size="sm" disabled={choosing} onClick={() => void choose()}>
        {choosing ? <Trans>Choosing…</Trans> : <Trans>Choose…</Trans>}
      </Button>
      {value && (
        <Button variant="outline" size="sm" onClick={() => onSave("")}>
          <Trans>Use automatic</Trans>
        </Button>
      )}
    </div>
  );
}

export function PlaybackSection() {
  const { t } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
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

  return (
    <SettingsPanel>
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
      <AudioRows />
      <CrossfadeRows />
      <QueueRows />
      <SettingRow
        id="cdm-path"
        stacked
        title={t`Widevine CDM location`}
        description={
          <Trans>
            Sona uses the <span className="text-ink">libwidevinecdm.so</span> from your installed
            Google Chrome. Choose a file only if Diagnostics can't find it. Applies the next time
            the playback host starts.
          </Trans>
        }
      >
        <CdmPathField value={s.cdmPath} onSave={(cdmPath) => save({ cdmPath })} />
      </SettingRow>
    </SettingsPanel>
  );
}
