import { plural } from "@lingui/core/macro";
import { useLingui } from "@lingui/react/macro";

import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { useCapabilities } from "@/shared/lib/capabilities";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";

import { AudioRows } from "./AudioRows";
import { CdmRow } from "./CdmRow";
import { CrossfadeRows } from "./CrossfadeRows";
import { LyricsRows } from "./LyricsRows";
import { PrivateRows } from "./PrivateRows";
import { QueueRows } from "./QueueRows";
import { QualityRow } from "./QualityRow";
import { useSettings, useUpdateSettings } from "./queries";
import {
  SELECT_WIDTH,
  SettingRow,
  SettingsGroup,
  SettingsLoading,
  SettingsPanel,
  SettingsSections,
  UnavailableRow,
} from "./SettingRow";

const idleChoices = [5, 10, 15, 30, 60];

/**
 * Settings → Playback: Widevine's state first (the player bar's Widevine
 * notice links here), then Sound, Listening, Lyrics and Advanced. Rows for
 * what this platform's playback can't do say so in their place; without
 * any playback, one line says so instead (lyrics still show on song pages).
 * The volume is the player bar's (and MPRIS's and `sona volume`'s), never
 * a row here.
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
  if (!s || !caps) return <SettingsLoading rows={5} />;
  const playback = caps.playback;
  if (!playback.available) {
    return (
      <SettingsSections>
        <SettingsPanel>
          <UnavailableRow
            id="playback"
            title={t`Playback isn't available on this platform yet`}
            reason={t`Browsing Apple Music and your library works as usual.`}
          />
        </SettingsPanel>
        <LyricsRows />
      </SettingsSections>
    );
  }
  const save = (patch: Partial<typeof s>) => update.mutate({ ...s, ...patch });
  const idle = idleChoices.includes(s.engineIdleMinutes)
    ? idleChoices
    : [...idleChoices, s.engineIdleMinutes].sort((a, b) => a - b);
  const cdm = playback.engine === "cdm";

  return (
    <SettingsSections>
      {cdm && (
        <SettingsPanel>
          <CdmRow />
        </SettingsPanel>
      )}
      <SettingsGroup id="playback-sound" title={t`Sound`}>
        <QualityRow />
        <AudioRows />
        <CrossfadeRows />
      </SettingsGroup>
      <SettingsGroup id="playback-listening" title={t`Listening`}>
        {playback.privateListening ? (
          <PrivateRows />
        ) : (
          <UnavailableRow
            id="private-listening"
            title={t({
              message: "Private listening",
              comment:
                "Apple Music's own name for the mode: plays aren't reported to Apple, so they don't shape recommendations or Replay.",
            })}
            reason={t`This platform's player reports plays to Apple itself, so Sona can't hold them back.`}
          />
        )}
        <QueueRows />
      </SettingsGroup>
      <LyricsRows />
      {cdm && (
        <SettingsGroup id="playback-advanced" title={t`Advanced`}>
          <SettingRow
            id="idle"
            title={t`Free memory when idle`}
            description={t`With nothing queued, Sona stops its playback host (Widevine and the audio pipeline) after this long. The next song then takes a few seconds longer to start. Applies the next time the host starts.`}
          >
            <Select
              value={String(s.engineIdleMinutes)}
              onValueChange={(v) => save({ engineIdleMinutes: Number(v) })}
            >
              <SelectTrigger id="idle" aria-describedby="idle-desc" className={SELECT_WIDTH}>
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
        </SettingsGroup>
      )}
    </SettingsSections>
  );
}
