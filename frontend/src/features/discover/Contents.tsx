import { Trans } from "@lingui/react/macro";
import { useId } from "react";

/** A place on the page: the element `id` to jump to and its title. */
export interface ContentsEntry {
  id: string;
  title: string;
}

export interface ContentsBand extends ContentsEntry {
  shelves: ContentsEntry[];
}

const jumpClassName =
  "rounded-sm text-left underline-offset-2 transition-colors duration-100 hover:underline";

/**
 * The page's contents: one line per band, its name and then its shelves,
 * each a jump to that place. It saves scrolling (and tabbing) through a
 * page of twenty-odd shelves to reach one near the end.
 */
export function Contents({
  bands,
  onJump,
}: {
  bands: readonly ContentsBand[];
  onJump: (id: string) => void;
}) {
  const heading = useId();
  return (
    <nav aria-labelledby={heading} className="mb-12">
      <h2 id={heading} className="mb-2 text-heading text-ink">
        <Trans>On this page</Trans>
      </h2>
      <ul>
        {bands.map((band) => (
          <li
            key={band.id}
            className="grid grid-cols-[9rem_minmax(0,1fr)] items-baseline gap-x-6 py-1"
          >
            <button
              type="button"
              onClick={() => onJump(band.id)}
              className={`${jumpClassName} justify-self-start text-ui font-medium text-ink`}
            >
              {band.title}
            </button>
            <ul aria-label={band.title} className="flex flex-wrap gap-x-2 gap-y-1">
              {band.shelves.map((shelf, i) => (
                <li key={shelf.id} className="flex min-w-0 items-baseline gap-2">
                  <button
                    type="button"
                    onClick={() => onJump(shelf.id)}
                    className={`${jumpClassName} text-sub text-ink-muted hover:text-ink`}
                  >
                    {shelf.title}
                  </button>
                  {i < band.shelves.length - 1 && (
                    <span aria-hidden className="text-sub text-ink-faint">
                      ·
                    </span>
                  )}
                </li>
              ))}
            </ul>
          </li>
        ))}
      </ul>
    </nav>
  );
}
