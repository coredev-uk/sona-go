import type { Item, Section } from "@bindings/services";
import { useLingui } from "@lingui/react/macro";

import { Shelf } from "@/shared/components/Shelf";

import { ItemShelf } from "./ItemShelf";
import { roomLink, type ItemLinkProps } from "./links";
import { LinkList } from "./LinkList";
import { playItem } from "./play";
import { StationList } from "./StationList";
import { TrackShelf } from "./TrackShelf";
import { WideRow } from "./WideRow";

/**
 * The hero card's shape on music.apple.com: its "flowcaseLockup" requests
 * the 4:1 subscriptionHero art cropped to 1060×608 ("sr").
 */
const HERO_RATIO = 1060 / 608;

interface EditorialSectionsProps {
  sections: readonly Section[];
  /** Where a section's "See all" goes (default: its room). */
  moreFor?: (section: Section) => ItemLinkProps | undefined;
  onOpen?: (item: Item) => void;
}

/**
 * Renders editorial and recommendation sections by kind: wide hero cards,
 * feature and cover shelves, track blocks, station lists, category bricks, link lists and
 * paragraphs. New, Radio, curators, groupings, multirooms and Home share it.
 */
export function EditorialSections({
  sections,
  moreFor = roomLink,
  onOpen,
}: EditorialSectionsProps) {
  const { t } = useLingui();
  return sections.map((s) => {
    const more = moreFor(s);
    switch (s.kind) {
      case "hero":
        return (
          <WideRow
            key={s.id}
            title={s.title}
            items={s.items}
            min={420}
            ratio={HERO_RATIO}
            crop
            onOpen={onOpen}
          />
        );
      case "bricks":
        return (
          <WideRow key={s.id} title={s.title} items={s.items} min={240} ratio={2} onOpen={onOpen} />
        );
      case "tracks":
        return <TrackShelf key={s.id} title={s.title} items={s.items} more={more} />;
      case "links":
        return <LinkList key={s.id} title={s.title} items={s.items} />;
      case "stations":
        return (
          <Shelf key={s.id} title={s.title} more={more}>
            <StationList
              label={s.title}
              stations={s.items.flatMap((i) => (i.station ? [i.station] : []))}
              onPlay={(station) => {
                const item = s.items.find((i) => i.station === station);
                if (item) playItem(item);
              }}
            />
          </Shelf>
        );
      case "text":
        return (
          <p
            key={s.id}
            className="mt-10 max-w-[72ch] text-ui text-pretty text-ink-muted first:mt-0"
          >
            {s.text}
          </p>
        );
      case "playlists":
        return (
          <ItemShelf key={s.id} title={t`Playlists`} items={s.items} more={more} onOpen={onOpen} />
        );
      case "feature":
        return (
          <ItemShelf
            key={s.id}
            title={s.title}
            items={s.items}
            more={more}
            min={208}
            onOpen={onOpen}
          />
        );
      default:
        return <ItemShelf key={s.id} title={s.title} items={s.items} more={more} onOpen={onOpen} />;
    }
  });
}
