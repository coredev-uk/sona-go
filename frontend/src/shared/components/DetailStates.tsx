import { EmptyState } from "@/shared/components/EmptyState";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { callFailure } from "@/shared/lib/errors";

/** The collection header and track rows at their real sizes while the detail loads. */
export function DetailSkeleton({ round }: { round?: boolean }) {
  return (
    <div aria-busy aria-label="Loading">
      <div className="mb-6 flex items-end gap-7 pt-1 pb-7">
        <div
          className={`size-[200px] shrink-0 bg-hover ${round ? "rounded-full" : "rounded-sm"}`}
        />
        <div className="flex-1 pb-1">
          <div className="h-8 w-2/5 rounded-sm bg-hover" />
          <div className="mt-3 h-4 w-1/4 rounded-sm bg-hover" />
          <div className="mt-2 h-3 w-1/3 rounded-sm bg-hover" />
          <div className="mt-6 flex gap-2">
            <div className="size-10 rounded-full bg-hover" />
            <div className="h-8 w-24 rounded-md bg-hover" />
          </div>
        </div>
      </div>
      {Array.from({ length: 8 }, (_, i) => (
        <div key={i} className="flex h-11 items-center gap-4 px-2">
          <div className="h-3 w-5 rounded-sm bg-hover" />
          <div
            className="h-3.5 rounded-sm bg-hover"
            style={{ width: `${30 + ((i * 17) % 30)}%` }}
          />
        </div>
      ))}
    </div>
  );
}

/** A detail page that failed: unavailable (not found) or an error with Retry. */
export function DetailError({
  noun,
  error,
  retry,
  retrying,
  back,
}: {
  noun: "album" | "artist" | "playlist" | "song" | "music video";
  error: unknown;
  retry: () => void;
  retrying: boolean;
  back: {
    label: string;
    to:
      "/library/albums" | "/library/artists" | "/library/playlists" | "/library/songs" | "/search";
  };
}) {
  const { code } = callFailure(error);
  if (code === "not_found") {
    return (
      <EmptyState title={`This ${noun} isn't available`} action={back} className="mt-2">
        It isn't in your library on this computer, and Apple Music didn't return it. It may not be
        offered in your storefront, or it was removed.
      </EmptyState>
    );
  }
  if (code === "not_signed_in") {
    return (
      <EmptyState
        title="Sign in to see this"
        action={{ label: "Sign in", to: "/settings", search: { section: "account" } }}
        className="mt-2"
      >
        Apple Music needs a signed-in account to show this {noun}.
      </EmptyState>
    );
  }
  return (
    <ErrorBlock
      className="mt-2"
      title={`Couldn't load this ${noun}`}
      error={error}
      onRetry={retry}
      retrying={retrying}
    />
  );
}
