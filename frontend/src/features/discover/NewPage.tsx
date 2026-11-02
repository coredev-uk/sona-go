import type { Section } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { useId } from "react";

import { EmptyState } from "@/shared/components/EmptyState";
import { PageHeader } from "@/shared/components/PageHeader";
import { EditorialSections } from "@/shared/items/EditorialSections";
import { cn } from "@/shared/lib/utils";

import { bandTitle, toBands } from "./bands";
import { Contents } from "./Contents";
import { FrontPage, FrontPageSkeleton } from "./FrontPage";
import { freshness } from "./freshness";
import { useEditorialPage } from "./queries";
import { PageError, ShelvesSkeleton } from "./states";

/**
 * Discover: Apple Music's editorial front page, set as Sona's own issue:
 * the featured cards as a lead and a list, the contents, then the shelves
 * in bands (songs and albums, playlists, radio, videos, browse).
 */
export function NewPage() {
  const { t } = useLingui();
  const q = useEditorialPage("new", "");
  return (
    <>
      <PageHeader
        title={t`Discover`}
        subline={
          q.data ? (
            freshness(q.data.fetchedAt, q.data.stale)
          ) : q.isPending ? (
            // Holds the freshness line's place, so nothing moves when it lands.
            <span aria-hidden className="inline-block h-3 w-28 rounded-sm bg-hover align-middle" />
          ) : undefined
        }
      />
      {q.isPending ? (
        <>
          <FrontPageSkeleton />
          <ContentsSkeleton />
          <div aria-hidden className="mb-5 border-t border-line pt-6">
            <div className="flex h-[30px] items-center">
              <div className="h-5 w-44 rounded-sm bg-hover" />
            </div>
          </div>
          <ShelvesSkeleton />
        </>
      ) : q.isError ? (
        <PageError
          title={t`Couldn't load what's new`}
          error={q.error}
          onRetry={() => void q.refetch()}
          retrying={q.isRefetching}
        />
      ) : q.data.sections.length ? (
        <Issue sections={q.data.sections} />
      ) : (
        <EmptyState frame="covers" title={t`Nothing here right now`}>
          <Trans>Apple Music has no music on this page at the moment.</Trans>
        </EmptyState>
      )}
    </>
  );
}

/** The contents' frame while the page loads: its heading and a line per band. */
function ContentsSkeleton() {
  return (
    <div aria-hidden className="mb-12">
      <div className="mb-2 flex h-[22px] items-center">
        <div className="h-4 w-28 rounded-sm bg-hover" />
      </div>
      {[0.7, 0.45, 0.3, 0.3, 0.25].map((w, i) => (
        <div key={i} className="grid h-7 grid-cols-[9rem_minmax(0,1fr)] items-center gap-x-6">
          <div className="h-3.5 w-24 rounded-sm bg-hover" />
          <div className="h-3 rounded-sm bg-hover" style={{ width: `${w * 100}%` }} />
        </div>
      ))}
    </div>
  );
}

function Issue({ sections }: { sections: readonly Section[] }) {
  const base = useId();
  const hero = sections.find((s) => s.kind === "hero");
  // What the front page shows isn't repeated in the cover shelves below it.
  const featured = new Set(hero?.items.flatMap((i) => (i.id ? [`${i.kind}:${i.id}`] : [])));
  const below = sections.flatMap((s) => {
    if (s === hero) return [];
    if (s.kind !== "shelf" && s.kind !== "feature") return [s];
    const items = s.items.filter((i) => !featured.has(`${i.kind}:${i.id}`));
    return [items.length ? { ...s, items } : s];
  });
  const bands = toBands(below).map((band) => ({
    ...band,
    anchor: `${base}${band.id}`,
    title: bandTitle(band.id),
  }));
  const jump = (id: string) => {
    const target = document.getElementById(id);
    target?.scrollIntoView({ block: "start" });
    target?.focus({ preventScroll: true });
  };
  return (
    <>
      {hero && <FrontPage items={hero.items} />}
      {bands.length > 1 && (
        <Contents
          bands={bands.map((band) => ({
            id: band.anchor,
            title: band.title,
            shelves: band.sections.flatMap((s, i) =>
              s.title ? [{ id: `${band.anchor}-${i}`, title: s.title }] : [],
            ),
          }))}
          onJump={jump}
        />
      )}
      <div>
        {bands.map((band) => (
          <section
            key={band.id}
            id={band.anchor}
            tabIndex={-1}
            aria-labelledby={`${band.anchor}-title`}
            className="mt-14 scroll-mt-6 border-t border-line pt-6 outline-none first:mt-0"
          >
            <h2 id={`${band.anchor}-title`} className="mb-5 text-page text-ink">
              {band.title}
            </h2>
            <div>
              {band.sections.map((s, i) => (
                <div
                  key={s.id}
                  id={`${band.anchor}-${i}`}
                  tabIndex={-1}
                  // An untitled link list continues the one above it.
                  className={cn(
                    "scroll-mt-6 outline-none",
                    !(s.kind === "links" && !s.title) && "mt-10 first:mt-0",
                  )}
                >
                  <EditorialSections sections={[s]} />
                </div>
              ))}
            </div>
          </section>
        ))}
      </div>
    </>
  );
}
