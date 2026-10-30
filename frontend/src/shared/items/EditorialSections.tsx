import type { Item, Section } from "@bindings/services";

import { ItemShelf } from "./ItemShelf";
import { roomLink, type ItemLinkProps } from "./links";
import { LinkList } from "./LinkList";
import { TrackShelf } from "./TrackShelf";
import { WideRow } from "./WideRow";

interface EditorialSectionsProps {
  sections: readonly Section[];
  /** Where a section's "See all" goes (default: its room). */
  moreFor?: (section: Section) => ItemLinkProps | undefined;
  onOpen?: (item: Item) => void;
}

/**
 * Renders editorial and recommendation sections by kind: wide hero cards,
 * feature and cover shelves, track blocks, category bricks, link lists and
 * paragraphs. New, Radio, curators, groupings, multirooms and Home share it.
 */
export function EditorialSections({
  sections,
  moreFor = roomLink,
  onOpen,
}: EditorialSectionsProps) {
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
            ratio={2.4}
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
      case "text":
        return (
          <p
            key={s.id}
            className="mt-10 max-w-[72ch] text-ui text-pretty text-ink-muted first:mt-0"
          >
            {s.text}
          </p>
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
