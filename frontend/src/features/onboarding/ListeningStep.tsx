import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";

import { isForced, setPrivate, usePrivate } from "@/features/private";
import { useSettings, useUpdateSettings } from "@/features/settings";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { useCapabilities } from "@/shared/lib/capabilities";
import { Button } from "@/shared/ui/button";

import { Panel, StepActions, StepHeader, SwitchRow } from "./parts";

/**
 * The listening step: private listening (PrivateService, as the player
 * bar's switch), Sound Check and crossfade (the settings service, as
 * Settings → Playback), each shown where the platform's playback supports
 * it. Every switch saves at once.
 */
export function ListeningStep({ onNext }: { onNext: () => void }) {
  const { t } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  const playback = useCapabilities()?.playback;
  const priv = usePrivate((s) => s.state);
  const s = settings.data;
  const forced = isForced(priv);

  const privateDescription = forced
    ? t`On because Sona was started with SONA_PRIVATE=1. Start Sona without it to turn private listening off.`
    : priv?.source === "development"
      ? t`This development build of Sona starts with private listening on. Switching it here lasts until Sona quits.`
      : t`Plays stay out of your Apple Music listening history and recommendations, and aren't scrobbled, shown on Discord or added to History.`;
  const seconds = s?.crossfade.seconds ?? 0;

  return (
    <>
      <StepHeader title={t`Listening`}>
        <Trans>
          How Sona plays your music and what it tells Apple about it. Settings → Playback has these
          and more.
        </Trans>
      </StepHeader>
      {s ? (
        <Panel>
          <SwitchRow
            id="setup-private"
            title={t({
              message: "Private listening",
              comment:
                "Setting name. No Apple Music web string; Apple's apps offer the inverse, 'Use Listening History'.",
            })}
            description={privateDescription}
            checked={priv?.on ?? false}
            disabled={!priv || forced}
            onCheckedChange={(on) => void setPrivate(on)}
          />
          {playback?.soundCheck && (
            <SwitchRow
              id="setup-sound-check"
              title={t`Sound Check`}
              description={t`Plays every song at about the same loudness, using the loudness Apple measured for it.`}
              checked={s.audio.soundCheck}
              onCheckedChange={(soundCheck) =>
                update.mutate({ ...s, audio: { ...s.audio, soundCheck } })
              }
            />
          )}
          {playback?.crossfade && (
            <SwitchRow
              id="setup-crossfade"
              title={t`Crossfade`}
              description={t`Fades each song into the next over ${plural(seconds, { one: "# second", other: "# seconds" })}, except between songs that follow each other on an album.`}
              checked={s.crossfade.enabled}
              onCheckedChange={(enabled) =>
                update.mutate({ ...s, crossfade: { ...s.crossfade, enabled } })
              }
            />
          )}
        </Panel>
      ) : settings.error ? (
        <ErrorBlock
          title={t`Couldn't read Sona's settings`}
          error={settings.error}
          onRetry={() => void settings.refetch()}
          retrying={settings.isRefetching}
        />
      ) : (
        <div aria-busy className="h-56 rounded-lg bg-surface" />
      )}
      <StepActions>
        <Button size="sm" onClick={onNext}>
          <Trans>Continue</Trans>
        </Button>
      </StepActions>
    </>
  );
}
