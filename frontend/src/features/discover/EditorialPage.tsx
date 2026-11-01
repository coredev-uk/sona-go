import { Trans, useLingui } from "@lingui/react/macro";
import type { EditorialPage as Page } from "@bindings/services";
import { getRouteApi } from "@tanstack/react-router";
import { useRef, useState } from "react";

import { CollectionHeader } from "@/shared/components/CollectionHeader";
import { EmptyState } from "@/shared/components/EmptyState";
import { PageHeader } from "@/shared/components/PageHeader";
import { artAspect, artUrl } from "@/shared/lib/art";
import { useElementWidth } from "@/shared/lib/useCoverColumns";
import { EditorialSections } from "@/shared/items/EditorialSections";

import { useEditorialPage, type EditorialKind } from "./queries";
import { freshness } from "./freshness";
import { PageError, ShelvesSkeleton } from "./states";

const routes = {
  curator: getRouteApi("/curator/$id"),
  multiroom: getRouteApi("/multiroom/$id"),
  grouping: getRouteApi("/grouping/$id"),
};

/** A curator (genre, radio show, brand), multiroom (campaign) or grouping (hub) page. */
export function EditorialRoutePage({ kind }: { kind: Exclude<EditorialKind, "new"> }) {
  const { id } = routes[kind].useParams();
  const q = useEditorialPage(kind, id);
  const { t } = useLingui();
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
        title={
          {
            curator: t`Couldn't load this curator`,
            multiroom: t`Couldn't load this collection`,
            grouping: t`Couldn't load this page`,
          }[kind]
        }
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
        <EmptyState frame="covers" title={t`Nothing here right now`}>
          <Trans>Apple Music has no music on this page at the moment.</Trans>
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
        header={{ backdrop: "", colors: p.color ? [p.color] : [], motion: "" }}
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

/**
 * A campaign banner at its own shape, at most 256px tall, letterboxed on
 * its own colour when that cap bites; never cropped.
 */
function Banner({ src, color }: { src: string; color: string }) {
  const ref = useRef<HTMLDivElement>(null);
  // Rounded up to 40px steps, so resizing the window reuses cached sizes.
  const width = Math.ceil(useElementWidth(ref) / 40) * 40;
  const [failed, setFailed] = useState(false);
  if (failed) return null;
  const aspect = artAspect(src);
  return (
    <div
      ref={ref}
      className="relative mb-6 max-h-64 w-full overflow-hidden rounded-sm bg-hover"
      style={{ aspectRatio: aspect, backgroundColor: color || undefined }}
    >
      {width > 0 && (
        <img
          src={artUrl(src, { width, height: Math.min(width / aspect, 256) }, "contain")}
          alt=""
          decoding="async"
          onError={() => setFailed(true)}
          className="absolute inset-0 size-full object-contain"
        />
      )}
    </div>
  );
}
