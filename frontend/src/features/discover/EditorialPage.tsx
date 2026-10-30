import type { EditorialPage as Page } from "@bindings/services";
import { getRouteApi } from "@tanstack/react-router";
import { useState } from "react";

import { CollectionHeader } from "@/shared/components/CollectionHeader";
import { EmptyState } from "@/shared/components/EmptyState";
import { PageHeader } from "@/shared/components/PageHeader";
import { artAt } from "@/shared/lib/art";
import { EditorialSections } from "@/shared/items/EditorialSections";

import { useEditorialPage, type EditorialKind } from "./queries";
import { freshness } from "./freshness";
import { PageError, ShelvesSkeleton } from "./states";

const nouns: Record<Exclude<EditorialKind, "new">, string> = {
  curator: "this curator",
  multiroom: "this collection",
  grouping: "this page",
};

const routes = {
  curator: getRouteApi("/curator/$id"),
  multiroom: getRouteApi("/multiroom/$id"),
  grouping: getRouteApi("/grouping/$id"),
};

/** A curator (genre, radio show, brand), multiroom (campaign) or grouping (hub) page. */
export function EditorialRoutePage({ kind }: { kind: Exclude<EditorialKind, "new"> }) {
  const { id } = routes[kind].useParams();
  const q = useEditorialPage(kind, id);
  if (q.isPending) {
    return (
      <>
        <div aria-hidden className="mb-6 h-9 w-72 rounded-sm bg-hover" />
        <ShelvesSkeleton />
      </>
    );
  }
  if (q.isError) {
    return (
      <PageError
        what={nouns[kind]}
        error={q.error}
        onRetry={() => void q.refetch()}
        retrying={q.isRefetching}
      />
    );
  }
  const p = q.data;
  return (
    <>
      <Header page={p} />
      {p.sections.length ? (
        <EditorialSections sections={p.sections} />
      ) : (
        <EmptyState frame="covers" title="Nothing here right now">
          Apple Music has no music on this page at the moment.
        </EmptyState>
      )}
    </>
  );
}

export const CuratorPage = () => <EditorialRoutePage kind="curator" />;
export const MultiroomPage = () => <EditorialRoutePage kind="multiroom" />;
export const GroupingPage = () => <EditorialRoutePage kind="grouping" />;

function Header({ page: p }: { page: Page }) {
  const meta = freshness(p.fetchedAt, p.stale);
  // Curators have a square logo: the collection header. Campaigns and hubs
  // have a banner or nothing.
  if (p.art.large) {
    return (
      <CollectionHeader
        title={p.title}
        art={p.art.large}
        byline={p.subtitle || undefined}
        meta={p.stale ? meta : undefined}
        description={p.notes || undefined}
        header={{ backdrop: "", colors: p.color ? [p.color] : [] }}
      />
    );
  }
  return (
    <>
      {p.wide.large && <Banner src={p.wide.large} color={p.color} />}
      <PageHeader title={p.title} subline={p.stale ? meta : p.subtitle || undefined} />
      {p.notes && (
        <p className="-mt-3 mb-8 max-w-[72ch] text-ui text-pretty text-ink-muted">{p.notes}</p>
      )}
    </>
  );
}

/** A campaign banner, letterboxed on its own colour; never cropped. */
function Banner({ src, color }: { src: string; color: string }) {
  const [failed, setFailed] = useState(false);
  if (failed) return null;
  return (
    <div
      className="relative mb-6 aspect-[4/1] max-h-64 w-full overflow-hidden rounded-sm bg-hover"
      style={{ backgroundColor: color || undefined }}
    >
      <img
        src={artAt(src, 1200)}
        alt=""
        decoding="async"
        onError={() => setFailed(true)}
        className="absolute inset-0 size-full object-contain"
      />
    </div>
  );
}
