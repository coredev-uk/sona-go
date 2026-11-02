import {
  CollaborationRole,
  type PlaylistCollaboration,
  type SocialProfile,
} from "@bindings/services";
import { plural, t } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { useState } from "react";

import { Artwork } from "@/shared/components/Artwork";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/shared/ui/dialog";

/** How many faces the header's stack shows before the count says the rest. */
const STACK = 4;

/**
 * A collaborative playlist's byline: the collaborators' faces and "Owner &
 * n others" ("You & n others" for the owner), opening the full list.
 */
export function Collaborators({ c }: { c: PlaylistCollaboration }) {
  const { t } = useLingui();
  const [open, setOpen] = useState(false);
  const owner = c.collaborators.find((p) => p.id === c.ownerId) ?? c.collaborators[0];
  const others = c.collaborators.length - 1;
  const lead = owner?.name || owner?.handle || "";
  const label =
    c.viewer === CollaborationRole.RoleOwner
      ? others > 0
        ? plural(others, { one: "You & # other", other: "You & # others" })
        : t`You`
      : others > 0
        ? plural(others, { one: `${lead} & # other`, other: `${lead} & # others` })
        : lead;
  return (
    <>
      <button
        type="button"
        aria-haspopup="dialog"
        title={t`View playlist collaborators`}
        onClick={() => setOpen(true)}
        className="group/collab -mx-1 inline-flex max-w-full items-center gap-2.5 rounded-md px-1 py-0.5 text-left"
      >
        <span aria-hidden className="flex shrink-0 -space-x-1.5">
          {c.collaborators.slice(0, STACK).map((p) => (
            <span key={p.id} className="rounded-full ring-2 ring-ground">
              <Artwork src={p.art.small} size={24} alt="" shape="round" eager />
            </span>
          ))}
        </span>
        <span className="truncate underline-offset-2 group-hover/collab:underline">{label}</span>
      </button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-[420px] gap-5 p-5">
          <DialogHeader className="gap-1.5">
            <DialogTitle className="text-heading text-ink">
              <Trans>Collaborators</Trans>
            </DialogTitle>
            <DialogDescription className="text-ui text-ink-muted">
              {[
                plural(c.collaborators.length, {
                  one: "# person can add songs to this playlist.",
                  other: "# people can add songs to this playlist.",
                }),
                roleLine(c.viewer),
              ]
                .filter(Boolean)
                .join(" ")}
            </DialogDescription>
          </DialogHeader>
          <PeopleList people={c.collaborators} ownerId={c.ownerId} />
          {c.former.length > 0 && (
            <section className="flex flex-col gap-2">
              <h3 className="text-sub font-semibold text-ink-muted">
                <Trans>Former collaborators</Trans>
              </h3>
              <PeopleList people={c.former} ownerId="" />
            </section>
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}

/** What the signed-in user is to the playlist ("" for a listener). */
function roleLine(viewer: CollaborationRole): string {
  if (viewer === CollaborationRole.RoleOwner) return t`You started this playlist.`;
  if (viewer === CollaborationRole.RoleCollaborator) return t`You're a collaborator.`;
  return "";
}

function PeopleList({ people, ownerId }: { people: SocialProfile[]; ownerId: string }) {
  return (
    <ul className="-mx-2 flex max-h-[min(60vh,420px)] flex-col overflow-y-auto">
      {people.map((p) => (
        <li key={p.id}>
          <Link
            to="/profile/$id"
            params={{ id: p.id }}
            className="flex items-center gap-3 rounded-md px-2 py-1.5 transition-colors duration-100 hover:bg-hover"
          >
            <Artwork src={p.art.small} size={40} alt="" shape="round" />
            <span className="min-w-0 flex-1">
              <span className="block truncate text-ui text-ink">{p.name || `@${p.handle}`}</span>
              {p.name && p.handle && (
                <span className="block truncate text-sub text-ink-muted">@{p.handle}</span>
              )}
            </span>
            {p.id === ownerId && (
              <span className="shrink-0 text-meta text-ink-faint">
                <Trans>Owner</Trans>
              </span>
            )}
          </Link>
        </li>
      ))}
    </ul>
  );
}
