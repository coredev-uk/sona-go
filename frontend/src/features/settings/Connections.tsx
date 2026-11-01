import type { ScrobbleAccount, Settings } from "@bindings/config";
import { plural, t } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { ScrobbleService, type ScrobblerStatus, type ScrobbleStatus } from "@bindings/services";
import { useQueryClient } from "@tanstack/react-query";
import { Browser } from "@wailsio/runtime";
import { CircleAlert, ExternalLink, LoaderCircle, ShieldAlert } from "lucide-react";
import { useId, useRef, useState, type FormEvent, type ReactNode, type Ref } from "react";
import { toast } from "sonner";

import { usePrivate } from "@/features/private";
import { useCapabilities } from "@/shared/lib/capabilities";
import { callFailure } from "@/shared/lib/errors";
import { Button } from "@/shared/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/shared/ui/dialog";
import { Input } from "@/shared/ui/input";
import { Switch } from "@/shared/ui/switch";

import { IntegrationsConnectionsRows } from "./IntegrationsRows";
import { useSettings, useUpdateSettings } from "./queries";
import { scrobbleStatusKey, useScrobbleStatus } from "./scrobble";
import { SettingsGroup, SettingsPanel, SettingsSections } from "./SettingRow";

type Provider = "lastfm" | "listenbrainz";

/** Brand names, never translated. */
const PROVIDER_NAMES = {
  // eslint-disable-next-line lingui/no-unlocalized-strings -- brand name
  lastfm: "Last.fm",
  // eslint-disable-next-line lingui/no-unlocalized-strings -- brand name
  listenbrainz: "ListenBrainz",
} as const satisfies Record<Provider, string>;

const listenBrainzTokenPage = "https://listenbrainz.org/settings/";

function pendingLine(n: number) {
  return plural(n, {
    one: "# song you played hasn't been sent yet.",
    other: "# songs you played haven't been sent yet.",
  });
}
/** What a connected (or broken) account is doing, in one sentence. */
function accountLine(p: Provider, st: ScrobblerStatus, acct: ScrobbleAccount | undefined) {
  const user = acct?.username;
  const pending = st.pending;
  const waiting =
    pending > 0
      ? " " + plural(pending, { one: "# song waiting to send.", other: "# songs waiting to send." })
      : "";
  if (st.needsReconnect) {
    const name = PROVIDER_NAMES[p];
    return t`${name} no longer accepts Sona's sign-in. Reconnect to keep scrobbling.` + waiting;
  }
  if (!acct?.enabled) {
    const line = user
      ? t`Connected as ${user}, paused. Songs you play now aren't sent.`
      : t`Connected, paused. Songs you play now aren't sent.`;
    return line + waiting;
  }
  return (user ? t`Scrobbling as ${user}.` : t`Scrobbling.`) + waiting;
}

/** One provider: its state on the left, connect or pause and disconnect on the right. */
function ProviderRow({
  provider,
  status,
  account,
  description,
  connect,
  busy,
  onToggle,
  onDisconnect,
  children,
  controlsRef,
}: {
  provider: Provider;
  status: ScrobblerStatus | undefined;
  account: ScrobbleAccount | undefined;
  description: string;
  connect: ReactNode;
  busy: boolean;
  onToggle: (on: boolean) => void;
  onDisconnect: () => void;
  children?: ReactNode;
  /** The row's buttons on the right, to put focus back on after its form closes. */
  controlsRef?: Ref<HTMLDivElement>;
}) {
  const id = `scrobble-${provider}`;
  const connected = status?.connected ?? false;
  return (
    <div className="flex flex-col gap-3 py-4">
      <div className="flex items-center justify-between gap-8">
        <div className="min-w-0">
          <p id={`${id}-title`} className="text-ui font-medium text-ink">
            {PROVIDER_NAMES[provider]}
          </p>
          <p
            id={`${id}-desc`}
            aria-live="polite"
            className="mt-0.5 max-w-[56ch] text-sub text-ink-muted"
          >
            {connected && status ? accountLine(provider, status, account) : description}
          </p>
        </div>
        <div ref={controlsRef} className="flex shrink-0 items-center gap-3">
          {connected ? (
            <>
              {status?.needsReconnect ? (
                connect
              ) : (
                <Switch
                  id={id}
                  aria-labelledby={`${id}-title`}
                  aria-describedby={`${id}-desc`}
                  checked={account?.enabled ?? false}
                  disabled={!account || busy}
                  onCheckedChange={onToggle}
                />
              )}
              <Button variant="ghost" size="sm" onClick={onDisconnect} disabled={busy}>
                <Trans>Disconnect</Trans>
              </Button>
            </>
          ) : (
            connect
          )}
        </div>
      </div>
      {children}
    </div>
  );
}

function errorLine(p: Provider, err: unknown): string | null {
  const { code, message } = callFailure(err);
  switch (code) {
    case "cancelled":
      return null;
    case "invalid_credentials":
      return p === "listenbrainz"
        ? t`ListenBrainz didn't accept that token. Copy it again from your ListenBrainz settings.`
        : t`Last.fm refused Sona's request. Try connecting again.`;
    case "timeout":
      return t`The Last.fm approval expired before it was confirmed. Connect again to get a new one.`;
    case "network": {
      const name = PROVIDER_NAMES[p];
      return t`Couldn't reach ${name}. Check your connection and try again.`;
    }
    case "not_configured":
      return t`This build of Sona has no Last.fm API key, so it can't connect to Last.fm.`;
  }
  return message;
}

/** Last.fm desktop authorisation: approve in the browser, Sona waits. */
function LastFMWaiting() {
  return (
    <div role="status" className="flex items-center gap-3 rounded-md bg-hover px-4 py-3">
      <LoaderCircle
        aria-hidden
        className="size-4 shrink-0 animate-spin text-ink-muted"
        strokeWidth={1.75}
      />
      <p className="min-w-0 flex-1 text-sub text-ink">
        <Trans>
          Allow Sona on the Last.fm page that opened in your browser. Sona continues by itself.
        </Trans>
      </p>
      <Button variant="ghost" size="sm" onClick={() => void ScrobbleService.CancelConnect()}>
        <Trans>Cancel</Trans>
      </Button>
    </div>
  );
}

/** The ListenBrainz user token form, shown under its row. */
function ListenBrainzForm({
  onDone,
  onCancel,
}: {
  onDone: (s: ScrobbleStatus) => void;
  onCancel: () => void;
}) {
  const { t } = useLingui();
  const id = useId();
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      onDone(await ScrobbleService.ConnectListenBrainz(token));
    } catch (err) {
      setError(errorLine("listenbrainz", err));
      setBusy(false);
    }
  };
  return (
    <form
      onSubmit={(e) => void submit(e)}
      onKeyDown={(e) => {
        if (e.key === "Escape") onCancel();
      }}
      className="flex flex-col gap-2"
    >
      <label htmlFor={`${id}-token`} className="text-sub font-medium text-ink">
        <Trans>User token</Trans>
      </label>
      <div className="flex gap-2">
        <Input
          id={`${id}-token`}
          type="password"
          autoComplete="off"
          spellCheck={false}
          autoFocus
          aria-describedby={`${id}-hint`}
          aria-invalid={error ? true : undefined}
          value={token}
          onChange={(e) => setToken(e.target.value)}
          className="h-8 bg-raised text-ui"
        />
        <Button type="submit" variant="outline" size="sm" disabled={busy || !token.trim()}>
          {busy ? t`Checking…` : t`Connect`}
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={onCancel}>
          <Trans>Cancel</Trans>
        </Button>
      </div>
      <p id={`${id}-hint`} className="text-meta text-ink-muted">
        <Trans>
          Copy it from{" "}
          <button
            type="button"
            onClick={() => void Browser.OpenURL(listenBrainzTokenPage)}
            className="inline-flex items-center gap-1 text-ink underline underline-offset-2 hover:no-underline"
          >
            your ListenBrainz settings
            <ExternalLink aria-hidden className="size-3" strokeWidth={1.75} />
          </button>
          . Sona checks it with ListenBrainz and keeps it in your keyring.
        </Trans>
      </p>
      {error && (
        <p role="alert" className="flex items-start gap-2 text-sub text-ink">
          <CircleAlert
            aria-hidden
            className="mt-0.5 size-4 shrink-0 text-danger"
            strokeWidth={1.75}
          />
          {error}
        </p>
      )}
    </form>
  );
}

function ScrobblingPanel() {
  const { t } = useLingui();
  const qc = useQueryClient();
  const settings = useSettings();
  const update = useUpdateSettings();
  const status = useScrobbleStatus();
  const [lbForm, setLbForm] = useState(false);
  const [confirm, setConfirm] = useState<Provider | null>(null);
  const [busy, setBusy] = useState<Provider | null>(null);
  const lbControls = useRef<HTMLDivElement>(null);
  // The form's Connect button goes with it: focus would fall to <body>, so
  // it goes to the row's Connect (cancelled) or its switch (connected).
  const closeLbForm = () => {
    setLbForm(false);
    setTimeout(() => {
      const now = document.activeElement;
      if (now && now !== document.body) return;
      lbControls.current?.querySelector<HTMLElement>("button:not(:disabled)")?.focus();
    }, 0);
  };

  const s = settings.data;
  const st = status.data;
  const accounts: Record<Provider, ScrobbleAccount | undefined> = {
    lastfm: s?.scrobbling.lastfm,
    listenbrainz: s?.scrobbling.listenbrainz,
  };
  const toggle = (p: Provider, enabled: boolean) => {
    if (!s) return;
    const scrobbling: Settings["scrobbling"] = {
      ...s.scrobbling,
      [p]: { ...s.scrobbling[p], enabled },
    };
    update.mutate({ ...s, scrobbling });
  };
  const connectLastFM = async () => {
    setBusy("lastfm");
    try {
      qc.setQueryData(scrobbleStatusKey, await ScrobbleService.ConnectLastFM());
    } catch (err) {
      const line = errorLine("lastfm", err);
      if (line) toast.error(t`Couldn't connect Last.fm`, { description: line });
    } finally {
      setBusy(null);
    }
  };
  const disconnect = async (p: Provider) => {
    setBusy(p);
    try {
      qc.setQueryData(scrobbleStatusKey, await ScrobbleService.Disconnect(p));
      setConfirm(null);
    } catch (err) {
      const name = PROVIDER_NAMES[p];
      toast.error(t`Couldn't disconnect ${name}`, { description: callFailure(err).message });
    } finally {
      setBusy(null);
    }
  };
  const askDisconnect = (p: Provider) => {
    if ((st?.[p].pending ?? 0) > 0) setConfirm(p);
    else void disconnect(p);
  };

  const lastfm = st?.lastfm;
  const lb = st?.listenbrainz;
  const lastfmConnecting = !!lastfm?.connecting || busy === "lastfm";
  const anyConnected = !!lastfm?.connected || !!lb?.connected;
  const statusError = status.error ? callFailure(status.error).message : "";
  const confirmName = confirm ? PROVIDER_NAMES[confirm] : "";
  const privateOn = usePrivate((p) => p.state?.on ?? false);

  return (
    <section aria-labelledby="scrobbling-title" className="flex flex-col gap-3">
      <div>
        <h3 id="scrobbling-title" className="text-ui font-semibold text-ink">
          <Trans>Scrobbling</Trans>
        </h3>
        <p className="mt-0.5 max-w-[64ch] text-sub text-ink-muted">
          <Trans>
            A song counts once you've played half of it or four minutes. Radio and songs of 30
            seconds or less aren't sent. Songs you play offline are sent when you're back online.
          </Trans>
        </p>
        {privateOn && (
          <p className="mt-1 max-w-[64ch] text-sub text-ink">
            <Trans>Nothing is scrobbled while private listening is on.</Trans>
          </p>
        )}
      </div>
      <SettingsPanel>
        <ProviderRow
          provider="lastfm"
          status={lastfm}
          account={accounts.lastfm}
          busy={busy === "lastfm"}
          description={
            lastfm && !lastfm.available
              ? t`This build of Sona has no Last.fm API key, so it can't connect to Last.fm.`
              : t`Adds each song you play to your Last.fm profile.`
          }
          connect={
            <Button
              variant="outline"
              size="sm"
              disabled={!lastfm?.available || lastfmConnecting}
              onClick={() => void connectLastFM()}
            >
              {lastfm?.needsReconnect ? t`Reconnect` : t`Connect`}
              <ExternalLink strokeWidth={1.75} />
            </Button>
          }
          onToggle={(on) => toggle("lastfm", on)}
          onDisconnect={() => askDisconnect("lastfm")}
        >
          {lastfmConnecting && <LastFMWaiting />}
        </ProviderRow>
        <ProviderRow
          provider="listenbrainz"
          status={lb}
          account={accounts.listenbrainz}
          busy={busy === "listenbrainz"}
          description={t`Adds each song you play to your public ListenBrainz history.`}
          connect={
            !lbForm && (
              <Button variant="outline" size="sm" disabled={!lb} onClick={() => setLbForm(true)}>
                {lb?.needsReconnect ? t`Reconnect` : t`Connect`}
              </Button>
            )
          }
          onToggle={(on) => toggle("listenbrainz", on)}
          onDisconnect={() => askDisconnect("listenbrainz")}
          controlsRef={lbControls}
        >
          {lbForm && (
            <ListenBrainzForm
              onCancel={closeLbForm}
              onDone={(next) => {
                qc.setQueryData(scrobbleStatusKey, next);
                closeLbForm();
              }}
            />
          )}
        </ProviderRow>
      </SettingsPanel>
      {status.error && (
        <p role="alert" className="flex items-start gap-2 text-sub text-ink">
          <CircleAlert
            aria-hidden
            className="mt-0.5 size-4 shrink-0 text-danger"
            strokeWidth={1.75}
          />
          <Trans>Couldn't read the scrobbler connections: {statusError}</Trans>
        </p>
      )}
      {st?.unencrypted && anyConnected && (
        <div
          role="alert"
          className="flex items-start gap-3 rounded-lg border border-control-line bg-raised px-4 py-3"
        >
          <ShieldAlert
            aria-hidden
            className="mt-0.5 size-4 shrink-0 text-danger"
            strokeWidth={1.75}
          />
          <div className="text-sub text-ink">
            <p className="font-medium">
              <Trans>Your scrobbler sign-ins are stored unencrypted</Trans>
            </p>
            <p className="mt-0.5 text-ink-muted">
              <Trans>
                No Secret Service keyring is running, so Sona keeps them in files only your user can
                read. Start GNOME Keyring or KWallet and Sona moves them into the keyring.
              </Trans>
            </p>
          </div>
        </div>
      )}
      <Dialog open={confirm !== null} onOpenChange={(open) => !open && setConfirm(null)}>
        <DialogContent showCloseButton={false} className="max-w-md">
          {confirm && (
            <>
              <DialogHeader>
                <DialogTitle>
                  <Trans>Disconnect {confirmName}?</Trans>
                </DialogTitle>
                <DialogDescription>
                  {pendingLine(st?.[confirm].pending ?? 0)}{" "}
                  <Trans>Sona discards them and removes its sign-in from your keyring.</Trans>
                </DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <Button variant="ghost" onClick={() => setConfirm(null)}>
                  <Trans>Keep connected</Trans>
                </Button>
                <Button
                  variant="destructive"
                  onClick={() => void disconnect(confirm)}
                  disabled={busy === confirm}
                >
                  {busy === confirm ? t`Disconnecting…` : t`Disconnect`}
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </section>
  );
}

export function ConnectionsSection() {
  const { t } = useLingui();
  const discord = useCapabilities()?.discordPresence ?? false;
  return (
    <SettingsSections>
      <ScrobblingPanel />
      {discord && (
        <SettingsGroup id="discord-title" title={t`Discord`}>
          <IntegrationsConnectionsRows />
        </SettingsGroup>
      )}
    </SettingsSections>
  );
}
