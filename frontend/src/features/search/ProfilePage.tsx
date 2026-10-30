import type { Item } from "@bindings/services";
import { getRouteApi } from "@tanstack/react-router";
import { BadgeCheck, Lock } from "lucide-react";

import { Artwork } from "@/shared/components/Artwork";
import { EmptyState } from "@/shared/components/EmptyState";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { Shelf } from "@/shared/components/Shelf";
import { ItemGrid } from "@/shared/items/ItemGrid";

import { useProfile } from "./queries";

const route = getRouteApi("/profile/$id");

const noArt = { small: "", medium: "", large: "" };

/** An Apple Music listener's profile: who they are and their public playlists. */
export function ProfilePage() {
  const { id } = route.useParams();
  const res = useProfile(id);

  if (res.error) {
    return (
      <ErrorBlock
        title="Couldn't load this profile"
        error={res.error}
        onRetry={() => void res.refetch()}
        retrying={res.isRefetching}
      />
    );
  }
  if (!res.data) {
    return (
      <div aria-busy className="flex items-center gap-6">
        <div className="size-32 rounded-full bg-hover" />
        <div className="flex flex-col gap-2">
          <div className="h-8 w-56 rounded-sm bg-hover" />
          <div className="h-4 w-28 rounded-sm bg-hover" />
        </div>
      </div>
    );
  }
  const { profile, playlists } = res.data;
  const items: Item[] = playlists.map(
    (p) =>
      ({
        kind: "playlist",
        id: p.id,
        name: p.name,
        subtitle: p.curatorName,
        caption: "",
        art: p.art,
        wide: noArt,
        color: "",
        explicit: false,
        url: "",
        playlist: p,
      }) as Item,
  );
  return (
    <>
      <header className="mb-10 flex items-center gap-6">
        <Artwork src={profile.art.large} size={128} alt="" shape="round" eager />
        <div className="min-w-0">
          <h1 className="flex items-center gap-2 text-display text-ink">
            <span className="truncate">{profile.name}</span>
            {profile.verified && (
              <BadgeCheck
                aria-label="Verified"
                className="size-6 shrink-0 text-ink-muted"
                strokeWidth={1.75}
              />
            )}
          </h1>
          <p className="mt-1 flex items-center gap-1.5 text-sub text-ink-muted">
            <span>@{profile.handle}</span>
            {profile.private && (
              <>
                <span aria-hidden>·</span>
                <Lock aria-hidden className="size-3.5" strokeWidth={1.75} />
                <span>Private profile</span>
              </>
            )}
          </p>
        </div>
      </header>
      {items.length ? (
        <Shelf title="Public playlists">
          <ItemGrid items={items} label={`Playlists shared by ${profile.name}`} />
        </Shelf>
      ) : (
        <EmptyState title="No public playlists">
          {profile.private
            ? `${profile.name} keeps their profile private, so only followers see what they share.`
            : `${profile.name} hasn't shared any playlists on their profile.`}
        </EmptyState>
      )}
    </>
  );
}
