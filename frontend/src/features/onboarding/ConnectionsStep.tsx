import { Trans, useLingui } from "@lingui/react/macro";
import { useQueryClient } from "@tanstack/react-query";
import { CircleCheck, ExternalLink } from "lucide-react";
import { useState, type ReactNode } from "react";

import { usePrivate } from "@/features/private";
import {
  LastFMWaiting,
  ListenBrainzForm,
  PROVIDER_NAMES,
  scrobbleStatusKey,
  useConnectLastFM,
  useIntegrationsStatus,
  useScrobbleStatus,
  useSettings,
  useUpdateSettings,
} from "@/features/settings";
import { useCapabilities } from "@/shared/lib/capabilities";
import { Button } from "@/shared/ui/button";

import { SwitchList, SwitchRow } from "./parts";

/** A service: its name and one line on the left, connecting or its state on the right. */
function ServiceRow({
  id,
  name,
  line,
  action,
  children,
}: {
  id: string;
  name: string;
  line: string;
  action: ReactNode;
  children?: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3 py-3">
      <div className="flex items-center justify-between gap-6">
        <div className="min-w-0">
          <p id={`${id}-name`} className="text-ui font-medium text-ink">
            {name}
          </p>
          <p className="text-sub text-pretty text-ink-muted">{line}</p>
        </div>
        <div className="flex shrink-0 items-center">{action}</div>
      </div>
      {children}
    </div>
  );
}

function Connected() {
  return (
    <span className="flex items-center gap-1.5 text-sub text-ink">
      <CircleCheck aria-hidden className="size-3.5 text-positive" strokeWidth={1.75} />
      <Trans>Connected</Trans>
    </span>
  );
}

/**
 * Connections, all optional: Last.fm and ListenBrainz scrobbling (the same
 * browser approval and token form as Settings → Connections) and Discord
 * status. Once connected, a service says so; pausing and disconnecting
 * stay in Settings.
 */
export function ConnectionsStep() {
  const { t } = useLingui();
  const qc = useQueryClient();
  const st = useScrobbleStatus().data;
  const settings = useSettings();
  const update = useUpdateSettings();
  const discord = useIntegrationsStatus().data?.discord;
  const discordShown = useCapabilities()?.discordPresence ?? false;
  const privateOn = usePrivate((p) => p.state?.on ?? false);
  const lastfmConnect = useConnectLastFM();
  const [lbForm, setLbForm] = useState(false);
  const s = settings.data;

  const lastfm = st?.lastfm;
  const lb = st?.listenbrainz;
  const lastfmWaiting = !!lastfm?.connecting || lastfmConnect.busy;
  // One message for both services: who Sona scrobbles as.
  const scrobblingAs = (name: string | undefined) =>
    name ? t`Scrobbling as ${name}.` : t`Scrobbling.`;
  const configured = discord?.configured ?? false;

  return (
    <div className="flex flex-col gap-3">
      <SwitchList>
        <ServiceRow
          id="setup-lastfm"
          name={PROVIDER_NAMES.lastfm}
          line={
            lastfm?.connected && !lastfm.needsReconnect
              ? scrobblingAs(s?.scrobbling.lastfm.username)
              : lastfm && !lastfm.available
                ? t`This build of Sona can't connect to Last.fm.`
                : t`Adds each song you play to your Last.fm profile.`
          }
          action={
            lastfm?.connected && !lastfm.needsReconnect ? (
              <Connected />
            ) : (
              <Button
                id="setup-lastfm-connect"
                aria-labelledby={["setup-lastfm-connect", "setup-lastfm-name"].join(" ")}
                variant="outline"
                size="sm"
                disabled={!lastfm?.available || lastfmWaiting}
                onClick={() => void lastfmConnect.connect()}
              >
                {lastfm?.needsReconnect ? t`Reconnect` : t`Connect`}
                <ExternalLink strokeWidth={1.75} />
              </Button>
            )
          }
        >
          {lastfmWaiting && <LastFMWaiting />}
        </ServiceRow>
        <ServiceRow
          id="setup-listenbrainz"
          name={PROVIDER_NAMES.listenbrainz}
          line={
            lb?.connected && !lb.needsReconnect
              ? scrobblingAs(s?.scrobbling.listenbrainz.username)
              : t`Adds each song you play to your public ListenBrainz history.`
          }
          action={
            lb?.connected && !lb.needsReconnect ? (
              <Connected />
            ) : (
              !lbForm && (
                <Button
                  id="setup-listenbrainz-connect"
                  aria-labelledby={["setup-listenbrainz-connect", "setup-listenbrainz-name"].join(
                    " ",
                  )}
                  variant="outline"
                  size="sm"
                  disabled={!lb}
                  onClick={() => setLbForm(true)}
                >
                  {lb?.needsReconnect ? t`Reconnect` : t`Connect`}
                </Button>
              )
            )
          }
        >
          {lbForm && (
            <ListenBrainzForm
              onCancel={() => {
                setLbForm(false);
                // The form's buttons go with it: focus goes back to Connect.
                setTimeout(() => document.getElementById("setup-listenbrainz-connect")?.focus());
              }}
              onDone={(next) => {
                qc.setQueryData(scrobbleStatusKey, next);
                setLbForm(false);
              }}
            />
          )}
        </ServiceRow>
        {discordShown && (
          <SwitchRow
            id="setup-discord"
            label={t`Discord status`}
            hint={
              discord && !configured
                ? t`Not available in this build of Sona.`
                : t`Shows the song you're playing as your Discord activity.`
            }
            checked={configured && (s?.integrations.discordPresence ?? false)}
            disabled={!s || !configured}
            onChange={(discordPresence) => {
              if (s) update.mutate({ ...s, integrations: { ...s.integrations, discordPresence } });
            }}
          />
        )}
      </SwitchList>
      {privateOn && (
        <p className="text-sub text-ink">
          <Trans>Nothing is scrobbled or shown on Discord while private listening is on.</Trans>
        </p>
      )}
    </div>
  );
}
