import { useState } from "react";

import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Slider } from "@/shared/ui/slider";

import { useSettings, useUpdateSettings } from "./queries";
import { SettingRow, SettingsPanel } from "./SettingRow";

const idleChoices = [5, 10, 15, 30, 60];

/** A text setting that saves on Enter or when the field loses focus. */
function CdmPathField({ value, onSave }: { value: string; onSave: (v: string) => void }) {
  const [draft, setDraft] = useState<{ base: string; text: string }>({ base: value, text: value });
  const text = draft.base === value ? draft.text : value;
  const commit = () => {
    const v = text.trim();
    if (v !== value) onSave(v);
  };
  return (
    <div className="flex gap-2">
      <Input
        id="cdm-path"
        aria-describedby="cdm-path-desc"
        value={text}
        spellCheck={false}
        placeholder="Found automatically"
        onChange={(e) => setDraft({ base: value, text: e.target.value })}
        onBlur={commit}
        onKeyDown={(e) => e.key === "Enter" && commit()}
        className="h-8 flex-1 text-ui"
      />
      {value && (
        <Button variant="outline" size="sm" onClick={() => onSave("")}>
          Use automatic
        </Button>
      )}
    </div>
  );
}

export function PlaybackSection() {
  const settings = useSettings();
  const update = useUpdateSettings();
  if (settings.error) {
    return (
      <ErrorBlock
        title="Couldn't read Sona's settings"
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

  return (
    <SettingsPanel>
      <SettingRow
        id="quality"
        title="Audio quality"
        description="High quality streams 256 kbps AAC. Data saver uses 64 kbps, for slow or metered connections."
      >
        <Select
          value={String(s.audioQuality)}
          onValueChange={(v) => save({ audioQuality: Number(v) })}
        >
          <SelectTrigger id="quality" aria-describedby="quality-desc" className="h-8 w-44">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="256">High · 256 kbps</SelectItem>
            <SelectItem value="64">Data saver · 64 kbps</SelectItem>
          </SelectContent>
        </Select>
      </SettingRow>
      <SettingRow
        id="volume"
        title="Volume"
        description="The same volume as the slider in the player bar."
      >
        <div
          className="flex w-44 items-center gap-3"
          style={{ "--slider-range": "var(--ink-muted)" } as React.CSSProperties}
        >
          <Slider
            id="volume"
            aria-describedby="volume-desc"
            value={[Math.round(s.volume * 100)]}
            max={100}
            step={1}
            onValueCommit={([v]) => save({ volume: v / 100 })}
          />
          <span className="w-9 text-right text-meta text-ink-muted num">
            {Math.round(s.volume * 100)}%
          </span>
        </div>
      </SettingRow>
      <SettingRow
        id="idle"
        title="Stop the playback host when idle"
        description="The playback host holds the Widevine CDM and the audio pipeline. With nothing queued, it shuts down after this long to free memory; the next song takes a few seconds longer to start. Applies the next time it starts."
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
                After {m} minutes
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </SettingRow>
      <SettingRow
        id="cdm-path"
        stacked
        title="Widevine CDM location"
        description={
          <>
            Sona uses the <span className="text-ink">libwidevinecdm.so</span> from your installed
            Google Chrome. Set a path only if Diagnostics can't find it. Applies the next time the
            playback host starts.
          </>
        }
      >
        <CdmPathField value={s.cdmPath} onSave={(cdmPath) => save({ cdmPath })} />
      </SettingRow>
    </SettingsPanel>
  );
}
