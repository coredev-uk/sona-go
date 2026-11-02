import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";

import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { useCapabilities } from "@/shared/lib/capabilities";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";

import { AudioRows } from "./AudioRows";
import { CdmRow } from "./CdmRow";
import { CrossfadeRows } from "./CrossfadeRows";
import { PrivateRows } from "./PrivateRows";
import { QueueRows } from "./QueueRows";
import { QualityRow } from "./QualityRow";
import { useSettings, useUpdateSettings } from "./queries";
import {
  SELECT_WIDTH,
  SettingRow,
  SettingsGroup,
  SettingsPanel,
  SettingsSections,
} from "./SettingRow";

const idleChoices = [5, 10, 15, 30, 60];

/**
 * Settings → Playback: Widevine's state first (the player bar's Widevine
 * notice links here), then Sound, Listening and Advanced. Rows for what
 * this platform's playback can't do are left out; without any playback,
 * one line says so instead. The volume is the player bar's (and MPRIS's
 * and `sona volume`'s), never a row here.
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
  if (!s || !caps) return <SettingsPanel className="h-72" />;
  const playback = caps.playback;
  if (!playback.available) {
    return (
      <SettingsPanel>
        <div className="py-4">
          <p className="text-ui font-medium text-ink">
            <Trans>Playback isn't available on this platform yet</Trans>
          </p>
          <p className="mt-0.5 max-w-[56ch] text-sub text-ink-muted">
            <Trans>Browsing Apple Music and your library works as usual.</Trans>
          </p>
        </div>
      </SettingsPanel>
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
        {playback.privateListening && <PrivateRows />}
        <QueueRows />
      </SettingsGroup>
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
