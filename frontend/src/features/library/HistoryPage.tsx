import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { useCallback, useMemo, useState } from "react";

import { hasAccount, useAuth } from "@/features/auth";
import { playSongs } from "@/features/player";
import { EmptyState } from "@/shared/components/EmptyState";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { PageHeader } from "@/shared/components/PageHeader";
import { formatNumber } from "@/shared/lib/format";
import { TrackList } from "@/shared/tracks/TrackList";
import { Button } from "@/shared/ui/button";

import { skeletonRows } from "./gate";
import { dayLabel, historyGroups, type HistoryGroup } from "./history";
import { useHistory } from "./queries";
import { signInAction } from "./sign-in";

/** Days loaded at first, and added by "Show earlier days". */
const PAGE_DAYS = 30;

const columns = { number: "row", artwork: true, artist: true, album: true } as const;

function Day({ group, now }: { group: HistoryGroup; now: number }) {
  const { t } = useLingui();
  const label = dayLabel(group.day, now);
  const songs = group.songs.length;
  const plays = group.plays;
  // "12 songs, 15 plays · 3 from other devices": the play count only when songs repeated.
  const parts = [
    plays > songs
      ? t`${plural(songs, { one: "# song", other: "# songs" })}, ${plural(plays, { one: "# play", other: "# plays" })}`
      : plural(songs, { one: "# song", other: "# songs" }),
  ];
  if (group.day && group.elsewhere) {
    const elsewhere = formatNumber(group.elsewhere);
    parts.push(t`${elsewhere} from other devices`);
  }
  const play = useCallback((i: number) => playSongs(group.songs, i, t`History`), [group.songs, t]);
  const id = `history-${group.day}`;
  return (
    <section aria-labelledby={id} className="mb-10 last:mb-0">
      <div className="mb-2 flex flex-wrap items-baseline gap-x-3 gap-y-0.5 px-2">
        <h2 id={id} className="text-heading text-ink">
          {label}
        </h2>
        <p className="text-meta text-ink-faint num">{parts.join(" · ")}</p>
      </div>
      <TrackList songs={group.songs} columns={columns} label={t`Played ${label}`} onPlay={play} />
    </section>
  );
}

/**
 * What the account played, newest first and grouped by day: plays on this
 * computer with Apple's recently played from the account's other devices
 * merged in by Go (LibraryService.History).
 */
export function HistoryPage() {
  const { t } = useLingui();
  const [days, setDays] = useState(PAGE_DAYS);
  const q = useHistory(days);
  const auth = useAuth();
  const signedOut =
    !!auth.data && !hasAccount(auth.data.state) && auth.data.state !== "needs_reauth";
  const groups = useMemo(() => (q.data ? historyGroups(q.data) : []), [q.data]);
  // Day labels are relative to when the data arrived ("Today").
  const now = q.dataUpdatedAt;

  let subline: string | undefined;
  if (q.data) {
    if (q.data.apple) subline = t`On this computer and your other devices`;
    else if (signedOut) subline = t`On this computer`;
    else subline = t`On this computer only: Apple Music couldn't be reached for your other devices`;
  }

  let body;
  if (q.error && !q.data) {
    body = (
      <ErrorBlock
        title={t`Couldn't read your listening history`}
        error={q.error}
        onRetry={() => void q.refetch()}
        retrying={q.isRefetching}
      />
    );
  } else if (q.isPending) {
    body = (
      <section aria-busy aria-label={t`Loading your listening history`}>
        <div className="mb-2 flex h-[22px] items-center px-2">
          <div className="h-4 w-24 rounded-sm bg-hover" />
        </div>
        <TrackList songs={skeletonRows} columns={columns} label={t`History`} onPlay={() => {}} />
      </section>
    );
  } else if (!groups.length) {
    body = signedOut ? (
      <EmptyState title={t`Nothing played on this computer yet`} action={signInAction()}>
        <Trans>
          Songs you play in Sona are listed here by day. Sign in to add what you play on your other
          devices.
        </Trans>
      </EmptyState>
    ) : (
      <EmptyState title={t`Nothing played yet`}>
        <Trans>Songs you play here or in any Apple Music app are listed here by day.</Trans>
      </EmptyState>
    );
  } else {
    body = (
      <>
        {groups.map((g) => (
          <Day key={g.day} group={g} now={now} />
        ))}
        <footer className="mt-8 flex items-center gap-4 px-2">
          {q.data?.more ? (
            <Button
              variant="outline"
              size="sm"
              disabled={q.isFetching}
              onClick={() => setDays((d) => d + PAGE_DAYS)}
            >
              {q.isFetching ? <Trans>Loading…</Trans> : <Trans>Show earlier days</Trans>}
            </Button>
          ) : (
            <p className="text-meta text-ink-faint">
              <Trans>Plays on this computer are kept for a year.</Trans>
            </p>
          )}
        </footer>
      </>
    );
  }

  return (
    <>
      <PageHeader title={t`History`} subline={subline} />
      {body}
    </>
  );
}
