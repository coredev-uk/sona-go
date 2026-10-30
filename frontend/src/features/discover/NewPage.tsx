import { PageHeader } from "@/shared/components/PageHeader";
import { EditorialSections } from "@/shared/items/EditorialSections";

import { useEditorialPage } from "./queries";
import { freshness } from "./freshness";
import { PageError, ShelvesSkeleton } from "./states";

/** Discover: Apple Music's editorial front page (new releases, playlists, radio, videos). */
export function NewPage() {
  const q = useEditorialPage("new", "");
  return (
    <>
      <PageHeader
        title="Discover"
        subline={q.data ? freshness(q.data.fetchedAt, q.data.stale) : undefined}
      />
      {q.isPending ? (
        <ShelvesSkeleton hero />
      ) : q.isError ? (
        <PageError
          what="what's new"
          error={q.error}
          onRetry={() => void q.refetch()}
          retrying={q.isRefetching}
        />
      ) : (
        <EditorialSections sections={q.data.sections} />
      )}
    </>
  );
}
