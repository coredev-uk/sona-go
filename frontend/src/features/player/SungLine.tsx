import { MicVocal } from "lucide-react";
import { useMemo, useRef } from "react";

import { Sung } from "./LyricBody";
import { hasWordTiming } from "./lyric-words";
import { isSynced, shownLine } from "./lyrics";
import { useLyrics } from "./queries";
import { useLyricLangs, useSungLine } from "./useLyricsView";
import { useWordSync } from "./useWordSync";

const PLAIN = { translation: false, pronunciation: false };

/**
 * The line being sung, for the mini player: up to two lines of text
 * marked with the lyrics glyph, its words lit whole in amber as they are
 * sung (no fill: a glance needs no frames). Between lines, in a break and
 * for songs without synced lyrics it shows `fallback` (the album) instead.
 */
export function SungLine({ catalogId, fallback }: { catalogId: string; fallback: string }) {
  const lyrics = useLyrics(catalogId || undefined);
  const lines = useMemo(() => lyrics.data?.lines ?? [], [lyrics.data]);
  const { active } = useSungLine(catalogId, lines, isSynced(lyrics.data));
  const langs = useLyricLangs(lyrics.data);
  const ref = useRef<HTMLDivElement>(null);
  // `active` as the layout: the loop looks for each new line once React has drawn it.
  useWordSync(ref, catalogId, lines, hasWordTiming(lyrics.data), active, true);
  const line = active >= 0 ? lines[active] : undefined;

  return (
    <div ref={ref} aria-live="off">
      {line?.text ? (
        <p
          key={active}
          data-line={active}
          dir="auto"
          lang={langs.lead || undefined}
          title={line.text}
          className="line-clamp-2 text-sub text-ink"
        >
          <MicVocal
            aria-hidden
            strokeWidth={2}
            className="me-1 inline size-3 -translate-y-px text-ink-faint"
          />
          <Sung text={shownLine(line, PLAIN).lead} />
        </p>
      ) : (
        <p className="truncate text-sub text-ink-faint" title={fallback}>
          {fallback}
        </p>
      )}
    </div>
  );
}
