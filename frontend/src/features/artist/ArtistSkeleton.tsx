import { useLingui } from "@lingui/react/macro";
import { useRef } from "react";

import { useElementWidth } from "@/shared/lib/useCoverColumns";
import { TrackList } from "@/shared/tracks/TrackList";

import { ESSENTIAL_ROWS, layoutFor, TOP_ROWS } from "./artist";
import { ReleaseList } from "./ReleaseList";

const noop = () => {};
const songs = Array.from<undefined>({ length: TOP_ROWS });
const albums = Array.from<undefined>({ length: ESSENTIAL_ROWS });

/** A shelf heading's bar, in the 28px heading row with its 12px gap. */
function HeadingBar({ width }: { width: string }) {
  return (
    <div className="mb-3 flex h-7 items-center">
      <div className="h-4 rounded-sm bg-hover" style={{ width }} />
    </div>
  );
}

/**
 * The artist page's frame while it loads, at the final sizes: the header
 * (portrait, name, catalogue line, two lines of biography, the actions,
 * the latest release), then top songs and the essential albums as loading
 * rows of the real lists. Nothing moves when the data lands.
 */
export function ArtistSkeleton() {
  const { t } = useLingui();
  const ref = useRef<HTMLDivElement>(null);
  const width = useElementWidth(ref);
  const layout = layoutFor(width);
  const slip = (
    <div className="flex items-end gap-3">
      <div
        className="shrink-0 rounded-sm bg-hover"
        style={{ width: layout.slip, height: layout.slip }}
      />
      <div className="flex w-40 flex-col gap-1.5">
        <div className="h-3.5 w-full rounded-sm bg-hover" />
        <div className="h-3 w-3/4 rounded-sm bg-hover" />
        <div className="h-3 w-1/2 rounded-sm bg-hover" />
      </div>
    </div>
  );
  return (
    <div ref={ref} aria-busy aria-label={t`Loading`}>
      <div className="relative -mx-8 -mt-7 mb-8 px-8 pt-8 pb-7 max-[960px]:-mx-6 max-[960px]:px-6">
        <div className="flex items-end gap-7">
          <div
            className="shrink-0 rounded-full bg-hover"
            style={{ width: layout.portrait, height: layout.portrait }}
          />
          <div className="min-w-0 flex-1 pb-1">
            <div className="flex h-9 items-center">
              <div className="h-7 w-2/5 rounded-sm bg-hover" />
            </div>
            <div className="mt-1 flex h-4 items-center">
              <div className="h-3 w-1/4 rounded-sm bg-hover" />
            </div>
            {/* Two 18px lines of biography, then "Read more". */}
            <div className="mt-3 max-w-[32rem]">
              <div className="flex h-[18px] items-center">
                <div className="h-3 w-full rounded-sm bg-hover" />
              </div>
              <div className="flex h-[18px] items-center">
                <div className="h-3 w-4/5 rounded-sm bg-hover" />
              </div>
              <div className="mt-0.5 flex h-[18px] items-center">
                <div className="h-3 w-16 rounded-sm bg-hover" />
              </div>
            </div>
            <div className="mt-5 flex items-center gap-2">
              <div className="size-10 rounded-full bg-hover" />
              <div className="h-8 w-24 rounded-md bg-hover" />
              <div className="size-8 rounded-md bg-hover" />
              <div className="size-8 rounded-md bg-hover" />
              <div className="size-8 rounded-md bg-hover" />
            </div>
          </div>
          {layout.wide && <div className="w-[300px] shrink-0 pb-1">{slip}</div>}
        </div>
        {!layout.wide && <div className="mt-6">{slip}</div>}
      </div>
      {width > 0 && (
        <div
          aria-hidden
          className="grid"
          style={{
            gridTemplateColumns: layout.wide ? "minmax(0, 1fr) 300px" : "1fr",
            columnGap: 40,
          }}
        >
          <div>
            <HeadingBar width="6rem" />
            <TrackList
              songs={songs}
              columns={{ number: "row", artwork: true, album: true }}
              label=""
              onPlay={noop}
            />
          </div>
          <div className={layout.wide ? undefined : "mt-10"}>
            <HeadingBar width="8rem" />
            <ReleaseList albums={albums} label="" columns={layout.wide ? 1 : 2} />
          </div>
        </div>
      )}
    </div>
  );
}
