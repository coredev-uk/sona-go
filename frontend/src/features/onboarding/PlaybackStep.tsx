import { Trans, useLingui } from "@lingui/react/macro";

import {
  CrossfadeRows,
  PrivateRows,
  QualityRow,
  SettingsGroup,
  SoundCheckRow,
  useSettings,
} from "@/features/settings";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { useCapabilities } from "@/shared/lib/capabilities";

import { StepHeader } from "./parts";
import { WidevinePanel } from "./WidevinePanel";

/**
 * Playback: Widevine's download first, then how Sona sounds (quality,
 * Sound Check, crossfade) and private listening, as Settings → Playback
 * has them. Without playback on this platform only Widevine's panel says
 * so; rows the platform's playback can't do are left out.
 */
export function PlaybackStep() {
  const { t } = useLingui();
  const settings = useSettings();
  const playback = useCapabilities()?.playback;
  return (
    <>
      <StepHeader title={t`Playback`}>
        <Trans>
          What plays your music and how it sounds. Settings → Playback has these and more.
        </Trans>
      </StepHeader>
      <div className="flex flex-col gap-8">
        <WidevinePanel />
        {settings.error ? (
          <ErrorBlock
            title={t`Couldn't read Sona's settings`}
            error={settings.error}
            onRetry={() => void settings.refetch()}
            retrying={settings.isRefetching}
          />
        ) : !settings.data || !playback ? (
          <div aria-busy className="h-56 rounded-lg bg-surface" />
        ) : (
          playback.available && (
            <>
              <SettingsGroup id="setup-sound" title={t`Sound`}>
                <QualityRow />
                <SoundCheckRow />
                <CrossfadeRows />
              </SettingsGroup>
              {playback.privateListening && (
                <SettingsGroup id="setup-listening" title={t`Listening`}>
                  <PrivateRows />
                </SettingsGroup>
              )}
            </>
          )
        )}
      </div>
    </>
  );
}
