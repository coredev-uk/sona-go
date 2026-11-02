import type { Album, Artist, Header } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";

import { Artwork } from "@/shared/components/Artwork";
import { Backdrop } from "@/shared/components/Backdrop";
import { MotionCover } from "@/shared/components/MotionCover";
import { catalogueLine, longDate } from "@/shared/lib/format";
import { releaseBadge } from "@/shared/lib/release";

import { layoutFor, releaseTitle } from "./artist";

interface ArtistHeadProps {
  artist: Artist;
  header?: Header;
  /** The catalogue line: `Artist · Alternative · Abingdon, Oxfordshire, England`. */
  line: string;
  /** The biography's first paragraph, with "Read more" to the About band. */
  lead?: string;
  onReadMore?: () => void;
  latest?: Album;
  actions: ReactNode;
  /** The content width (layoutFor). */
  width: number;
}

/** The latest release as a slip: its cover, title and catalogue line, linking to the album. */
function LatestRelease({ album, size }: { album: Album; size: number }) {
  const { t } = useLingui();
  // Apple's " - Single" / " - EP" suffix moves to the catalogue line.
  const { title, kind: release } = releaseTitle(album.name);
  const kind =
    release === "ep" ? t`EP` : release === "single" || album.single ? t`Single` : t`Album`;
  const when = releaseBadge(album) ?? longDate(album.releaseDate, "short");
  return (
    <Link
      to="/album/$id"
      params={{ id: album.id }}
      className="group/slip flex min-w-0 items-end gap-3 rounded-sm outline-offset-4"
    >
      <Artwork src={album.art.medium} size={size} alt="" />
      <span className="flex min-w-0 flex-col">
        <span className="line-clamp-2 text-ui font-medium text-ink group-hover/slip:underline group-hover/slip:underline-offset-2">
          {title}
        </span>
        <span className="text-meta text-ink-muted">
          {[t`Latest release`, when].filter(Boolean).join(" · ")}
        </span>
        <span className="text-meta text-ink-muted">
          {catalogueLine(kind, "", album.trackCount)}
        </span>
      </span>
    </Link>
  );
}

/**
 * The artist's file: the round portrait, the name at 32/36, the catalogue
 * line, the biography's opening lines and the actions, on a wash of the
 * portrait's colour (Go's pre-blurred backdrop under the ground scrim).
 * The latest release stands in the header's second column at wide widths
 * and under the portrait and text otherwise. It bleeds to the content
 * edges.
 */
export function ArtistHead({
  artist,
  header,
  line,
  lead,
  onReadMore,
  latest,
  actions,
  width,
}: ArtistHeadProps) {
  const layout = layoutFor(width);
  const latestSlip = latest && <LatestRelease album={latest} size={layout.slip} />;
  return (
    <header className="relative -mx-8 -mt-7 mb-8 px-8 pt-8 pb-7 max-[960px]:-mx-6 max-[960px]:px-6">
      <Backdrop src={header?.backdrop} color={header?.colors[0]} tone="ground" />
      <div className="relative flex items-end gap-7">
        {header?.motion ? (
          <MotionCover
            motion={header.motion}
            art={artist.art.large}
            size={layout.portrait}
            videoSize={200}
            shape="round"
            alt=""
            eager
          />
        ) : (
          <Artwork src={artist.art.large} size={layout.portrait} alt="" shape="round" eager />
        )}
        <div className="min-w-0 flex-1 pb-1">
          <h1 className="line-clamp-2 text-display text-balance text-ink">{artist.name}</h1>
          <p className="mt-1 text-meta text-ink-muted">{line}</p>
          {lead && (
            <div className="mt-3 max-w-[32rem]">
              <p className="line-clamp-2 text-sub text-ink-muted">{lead}</p>
              {onReadMore && (
                <button
                  type="button"
                  onClick={onReadMore}
                  className="mt-0.5 rounded-sm text-sub font-medium text-ink-muted underline-offset-2 hover:text-ink hover:underline"
                >
                  <Trans>Read more</Trans>
                </button>
              )}
            </div>
          )}
          <div className="mt-5 flex flex-wrap items-center gap-2">{actions}</div>
        </div>
        {latestSlip && layout.wide && <div className="w-[300px] shrink-0 pb-1">{latestSlip}</div>}
      </div>
      {latestSlip && !layout.wide && <div className="relative mt-6">{latestSlip}</div>}
    </header>
  );
}
