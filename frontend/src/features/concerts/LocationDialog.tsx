import type { ConcertLocation, ConcertPlace } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { Check } from "lucide-react";
import { useRef, useState, type KeyboardEvent } from "react";

import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/shared/ui/dialog";
import { Input } from "@/shared/ui/input";

import { countryName } from "./dates";
import { usePlaces, useSetLocation } from "./queries";

/**
 * Where to look for concerts: a search over Sona's own list of cities, run
 * on this computer. The choice is saved here; Apple is told only the
 * area's four-character code with each listing.
 */
export function LocationDialog({
  open,
  onOpenChange,
  location,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  location: ConcertLocation | undefined;
}) {
  const { t } = useLingui();
  const [query, setQuery] = useState("");
  const term = query.trim();
  const places = usePlaces(term);
  const change = (next: boolean) => {
    if (!next) setQuery("");
    onOpenChange(next);
  };
  const set = useSetLocation(() => change(false));
  const list = useRef<HTMLUListElement>(null);

  const items = places.data ?? [];
  const buttons = () =>
    Array.from(list.current?.querySelectorAll<HTMLButtonElement>("button") ?? []);

  const onInputKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      buttons()[0]?.focus();
    } else if (e.key === "Enter" && term && items[0]) {
      e.preventDefault();
      set.mutate(items[0].id);
    }
  };
  const onListKey = (e: KeyboardEvent<HTMLUListElement>) => {
    const all = buttons();
    const at = all.indexOf(document.activeElement as HTMLButtonElement);
    if (e.key === "ArrowDown" && at < all.length - 1) {
      e.preventDefault();
      all[at + 1].focus();
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      if (at > 0) all[at - 1].focus();
      else
        (e.currentTarget.parentElement?.querySelector("input") as HTMLInputElement | null)?.focus();
    } else if (e.key === "Home" && all.length) {
      e.preventDefault();
      all[0].focus();
    } else if (e.key === "End" && all.length) {
      e.preventDefault();
      all[all.length - 1].focus();
    }
  };

  return (
    <Dialog open={open} onOpenChange={change}>
      <DialogContent className="gap-4 sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            <Trans>Location</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>
              Concerts are listed near the city you choose. Sona keeps your choice on this computer
              and tells Apple Music only the area it covers.
            </Trans>
          </DialogDescription>
        </DialogHeader>
        <div>
          <Input
            type="search"
            value={query}
            autoFocus
            autoComplete="off"
            spellCheck={false}
            aria-label={t`Search cities`}
            placeholder={t`Search cities`}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onInputKey}
            className="h-8"
          />
          {/* Only an announcement: the list below is the control. */}
          <p className="sr-only" role="status">
            {term && items.length === 0 && !places.isFetching ? t`No cities found` : ""}
          </p>
          {items.length > 0 ? (
            <ul
              ref={list}
              aria-label={term ? t`Cities that match` : t`Suggestions`}
              onKeyDown={onListKey}
              className="mt-3 max-h-72 overflow-y-auto"
            >
              {items.map((p) => (
                <PlaceRow
                  key={p.id}
                  place={p}
                  pending={set.isPending}
                  onPick={() => set.mutate(p.id)}
                />
              ))}
            </ul>
          ) : (
            term && (
              <p className="mt-3 text-sub text-ink-muted">
                <Trans>No cities found</Trans>. <Trans>Try the name of a bigger city nearby.</Trans>
              </p>
            )
          )}
        </div>
        {location && !location.isDefault && (
          <DialogFooter className="sm:justify-start">
            <Button
              variant="ghost"
              size="sm"
              disabled={set.isPending}
              onClick={() => set.mutate("")}
            >
              <Trans>Use my storefront’s main city</Trans>
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}

function PlaceRow({
  place,
  pending,
  onPick,
}: {
  place: ConcertPlace;
  pending: boolean;
  onPick: () => void;
}) {
  return (
    <li>
      <button
        type="button"
        disabled={pending}
        aria-current={place.selected || undefined}
        onClick={onPick}
        className={cn(
          "flex h-10 w-full items-center gap-3 rounded-md px-2 text-left text-ui text-ink hover:bg-hover focus-visible:bg-hover",
          place.selected && "bg-active hover:bg-active",
        )}
      >
        <span className="min-w-0 flex-1 truncate">
          {place.name}
          <span className="text-ink-muted">, {countryName(place.countryCode)}</span>
        </span>
        {place.selected && <Check aria-hidden className="size-4 shrink-0" strokeWidth={1.75} />}
      </button>
    </li>
  );
}
