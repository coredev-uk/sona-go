import type { Song, StationDetail, StationEvent, StationSeed } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { useQueryClient } from "@tanstack/react-query";
import { getRouteApi, Link } from "@tanstack/react-router";
import { CircleSlash } from "lucide-react";

import { ActionMenu } from "@/shared/actions/ActionMenu";
import { Artwork } from "@/shared/components/Artwork";
import { CollectionHeader } from "@/shared/components/CollectionHeader";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { formatTime } from "@/shared/lib/format";
import { useNowPlaying } from "@/shared/lib/now-playing";
import { useNow } from "@/shared/lib/useNow";
import { cn } from "@/shared/lib/utils";

import { playStation } from "./commands";
import { radioKeys, useStation } from "./queries";
import { onAir, progressOf, slotLabel, stationLine } from "./schedule";

const route = getRouteApi("/station/$id");

/**
 * A station: what it is and who runs it, play, and for a live station the
 * schedule from the programme on air. A station Sona cannot play says why
 * where the play control would be.
 */
export function StationPage() {
  const { t } = useLingui();
  const { id } = route.useParams();
  const q = useStation(id);
  if (q.isPending) {
    return (
      <div aria-busy aria-label={t`Loading`} className="flex items-end gap-7 pt-1 pb-7">
        <div className="size-[200px] rounded-sm bg-hover" />
        <div className="flex-1 pb-1">
          <div className="h-8 w-80 rounded-sm bg-hover" />
          <div className="mt-3 h-3 w-48 rounded-sm bg-hover" />
          <div className="mt-6 size-10 rounded-full bg-hover" />
        </div>
      </div>
    );
  }
  if (q.isError) {
    return (
      <ErrorBlock
        title={t`Couldn't load this station`}
        error={q.error}
        onRetry={() => void q.refetch()}
        retrying={q.isRefetching}
      />
    );
  }
  return <Station detail={q.data} />;
}

const LINK_CLS = "hover:underline hover:underline-offset-2";

/** What a track station was made from, as the header's byline: "Music like Song by Artist". */
function SeedLine({ seed }: { seed: StationSeed }) {
  const name =
    seed.kind === "song" ? (
      <Link to="/song/$id" params={{ id: seed.id }} className={LINK_CLS}>
        {seed.name}
      </Link>
    ) : seed.kind === "album" ? (
      <Link to="/album/$id" params={{ id: seed.id }} className={LINK_CLS}>
        {seed.name}
      </Link>
    ) : (
      <Link to="/artist/$id" params={{ id: seed.id }} className={LINK_CLS}>
        {seed.name}
      </Link>
    );
  const by = seed.artistId ? (
    <Link to="/artist/$id" params={{ id: seed.artistId }} className={LINK_CLS}>
      {seed.subtitle}
    </Link>
  ) : (
    seed.subtitle
  );
  if (seed.subtitle) {
    return seed.only ? (
      <Trans>
        <span className="text-ink-muted">Only songs by </span>
        {name}
        <span className="text-ink-muted"> by </span>
        {by}
      </Trans>
    ) : (
      <Trans>
        <span className="text-ink-muted">Music like </span>
        {name}
        <span className="text-ink-muted"> by </span>
        {by}
      </Trans>
    );
  }
  return seed.only ? (
    <Trans>
      <span className="text-ink-muted">Only songs by </span>
      {name}
    </Trans>
  ) : (
    <Trans>
      <span className="text-ink-muted">Music like </span>
      {name}
    </Trans>
  );
}

function Station({ detail }: { detail: StationDetail }) {
  const s = detail.station;
  const qc = useQueryClient();
  const byline = detail.seed ? (
    <SeedLine seed={detail.seed} />
  ) : s.showId ? (
    <Link to="/curator/$id" params={{ id: s.showId }} className={LINK_CLS}>
      {s.showName}
    </Link>
  ) : s.hostName ? (
    s.hostName
  ) : (
    s.tagline || undefined
  );
  // Playing takes the listed songs; the page then lists the ones after them.
  const play = () =>
    void playStation(s.id).then(
      (ok) => ok && void qc.invalidateQueries({ queryKey: radioKeys.station(s.id) }),
    );
  return (
    <>
      <CollectionHeader
        title={s.name}
        art={s.art.large}
        byline={byline}
        meta={stationLine(s)}
        description={s.notes || undefined}
        header={detail.header}
        actions={
          <>
            <PlayButtons label={s.name} disabled={!s.playable} onPlay={play} />
            <ActionMenu kind="station" target={s} name={s.name} variant="header" />
          </>
        }
      />
      {!s.playable && (
        <div
          role="note"
          className="mb-8 flex max-w-[72ch] items-start gap-3 rounded-lg border border-dashed border-control-line px-4 py-3.5"
        >
          <CircleSlash
            aria-hidden
            className="mt-0.5 size-4 shrink-0 text-ink-muted"
            strokeWidth={1.75}
          />
          <div>
            <p className="text-ui font-medium text-ink">
              <Trans>Sona can't play this station</Trans>
            </p>
            <p className="mt-0.5 text-sub text-ink-muted">{s.unavailable}</p>
          </div>
        </div>
      )}
      {detail.schedule.length > 0 && <Schedule stationId={s.id} events={detail.schedule} />}
      {detail.upcoming.length > 0 && <Upcoming songs={detail.upcoming} />}
    </>
  );
}

/**
 * The songs a track station plays first, in order: the batch Go holds for
 * it, so pressing Play starts with exactly these.
 */
function Upcoming({ songs }: { songs: Song[] }) {
  return (
    <section aria-labelledby="station-upcoming" className="max-w-[880px]">
      <h2 id="station-upcoming" className="mb-2 text-heading text-ink">
        <Trans>Plays first</Trans>
      </h2>
      <ol>
        {songs.map((song) => (
          <li
            key={song.id}
            className="grid h-11 grid-cols-[32px_minmax(0,1fr)_minmax(0,0.8fr)_auto] items-center gap-4 rounded-md px-2"
          >
            <Artwork src={song.art.small} size={32} alt="" />
            <div className="min-w-0">
              <p className="truncate text-ui text-ink">
                {song.albumId ? (
                  <Link
                    to="/album/$id"
                    params={{ id: song.albumId }}
                    search={{ song: song.id }}
                    className={LINK_CLS}
                  >
                    {song.name}
                  </Link>
                ) : (
                  song.name
                )}
              </p>
              <p className="truncate text-sub text-ink-muted">
                {song.artistId ? (
                  <Link to="/artist/$id" params={{ id: song.artistId }} className={LINK_CLS}>
                    {song.artistName}
                  </Link>
                ) : (
                  song.artistName
                )}
              </p>
            </div>
            <p className="truncate text-sub text-ink-muted">{song.albumName}</p>
            <p className="text-meta text-ink-muted num">{formatTime(song.durationMs)}</p>
          </li>
        ))}
      </ol>
    </section>
  );
}

/**
 * The live schedule as a timetable: time on the left, the programme on
 * the right; the one on air shows how far through its slot it is (amber
 * only while Sona is playing this station).
 */
function Schedule({ stationId, events }: { stationId: string; events: StationEvent[] }) {
  const { t } = useLingui();
  const now = useNow(30_000);
  const { current, playing } = useNowPlaying();
  const live = onAir(events, now);
  const upcoming = events.filter((e) => e.endMs > now);
  const tuned = current?.id === stationId && playing;
  return (
    <section aria-labelledby="station-schedule" className="max-w-[880px]">
      <h2 id="station-schedule" className="mb-2 text-heading text-ink">
        <Trans>Schedule</Trans>
      </h2>
      <ol>
        {upcoming.map((e) => {
          const isOn = e === live;
          return (
            <li
              key={e.id}
              aria-current={isOn ? "time" : undefined}
              className="grid grid-cols-[112px_48px_minmax(0,1fr)] items-start gap-4 border-t border-line py-3.5 first:border-t-0"
            >
              <div className="pt-0.5">
                <p
                  className={cn("text-meta num", isOn ? "font-medium text-ink" : "text-ink-muted")}
                >
                  {slotLabel(e, now)}
                </p>
                {isOn && (
                  <p className="mt-0.5 text-meta text-ink-muted">
                    <Trans>On air</Trans>
                  </p>
                )}
              </div>
              <Artwork src={e.art.small} size={48} alt="" />
              <div className="min-w-0">
                <p
                  className={cn(
                    "text-ui font-medium",
                    isOn && tuned ? "text-play-ink" : "text-ink",
                  )}
                >
                  {e.title}
                </p>
                {e.description && (
                  <p className="mt-0.5 line-clamp-2 max-w-[72ch] text-sub text-ink-muted">
                    {e.description}
                  </p>
                )}
                {isOn && (
                  <div
                    role="progressbar"
                    aria-label={t`Time through this programme`}
                    aria-valuemin={0}
                    aria-valuemax={100}
                    aria-valuenow={Math.round(progressOf(e, now) * 100)}
                    className="mt-2.5 h-0.5 w-full max-w-80 overflow-hidden rounded-full bg-active"
                  >
                    <div
                      className={cn("h-full", tuned ? "bg-play" : "bg-ink-muted")}
                      style={{ width: `${progressOf(e, now) * 100}%` }}
                    />
                  </div>
                )}
              </div>
            </li>
          );
        })}
      </ol>
    </section>
  );
}
