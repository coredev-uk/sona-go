import { plural, t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react/macro";
import { ScrobbleService } from "@bindings/services";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";

import { callFailure } from "@/shared/lib/errors";

export type Provider = "lastfm" | "listenbrainz";

/** Brand names, never translated. */
export const PROVIDER_NAMES = {
  // eslint-disable-next-line lingui/no-unlocalized-strings -- brand name
  lastfm: "Last.fm",
  // eslint-disable-next-line lingui/no-unlocalized-strings -- brand name
  listenbrainz: "ListenBrainz",
} as const satisfies Record<Provider, string>;

/** The scrobblers' live state; Go's scrobble:changed replaces it. */
export const scrobbleStatusKey = ["scrobble", "status"] as const;

export function useScrobbleStatus() {
  return useQuery({ queryKey: scrobbleStatusKey, queryFn: () => ScrobbleService.Status() });
}

/** "1 song", "12 songs". */
export function songs(n: number) {
  return plural(n, { one: "# song", other: "# songs" });
}

/** What went wrong connecting a scrobbler, in a sentence; null when the user cancelled. */
export function errorLine(p: Provider, err: unknown): string | null {
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

/**
 * Starts Last.fm's desktop authorisation (the browser opens; Go waits for
 * the approval) and stores the status it ends with. A failure other than a
 * cancel is a toast. `busy` covers the call; Go's `connecting` the wait.
 */
export function useConnectLastFM(): { connect: () => Promise<void>; busy: boolean } {
  const { t } = useLingui();
  const qc = useQueryClient();
  const [busy, setBusy] = useState(false);
  const connect = async () => {
    setBusy(true);
    try {
      qc.setQueryData(scrobbleStatusKey, await ScrobbleService.ConnectLastFM());
    } catch (err) {
      const line = errorLine("lastfm", err);
      if (line) toast.error(t`Couldn't connect Last.fm`, { description: line });
    } finally {
      setBusy(false);
    }
  };
  return { connect, busy };
}
