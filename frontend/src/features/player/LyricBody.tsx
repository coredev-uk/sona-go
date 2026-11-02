import { cn } from "@/shared/lib/utils";

import type { ShownLine, ShownText } from "./lyrics";

/**
 * Text with its timed words as spans for the word loop to light
 * (useWordSync: `data-wb`/`data-we` are the word's begin and end in ms).
 * Untimed text is plain.
 */
export function Sung({ text }: { text: ShownText }) {
  if (!text.words.length) return text.text;
  return text.words.map((w, i) => (
    <span key={i} className="lyric-word" data-wb={w.beginMs} data-we={w.endMs}>
      {w.text}
    </span>
  ));
}

/** The languages of a line's parts, for `lang` ("" when unknown). */
export interface LyricLangs {
  lead: string;
  pronunciation: string;
  translation: string;
}

/**
 * One line's text: the main vocal, then, smaller and dimmer, the
 * background vocals (sung, so in the vocal's weight), the romanisation and
 * the translation (read, so regular), each on its own line.
 * `subClassName` sets the smaller lines' size and colour.
 */
export function LyricBody({
  shown,
  langs,
  subClassName,
}: {
  shown: ShownLine;
  langs: LyricLangs;
  subClassName: string;
}) {
  const subCls = cn("block", subClassName);
  const bg = shown.background;
  return (
    <>
      <span className="block" lang={langs.lead || undefined}>
        <Sung text={shown.lead} />
      </span>
      {(bg.text || bg.words.length > 0) && (
        <span className={cn(subCls, "font-semibold")} lang={langs.lead || undefined}>
          <Sung text={bg} />
        </span>
      )}
      {shown.pronunciation && (
        <span className={cn(subCls, "font-normal")} lang={langs.pronunciation || undefined}>
          <Sung text={shown.pronunciation} />
        </span>
      )}
      {shown.translation && (
        <span className={cn(subCls, "font-normal")} lang={langs.translation || undefined}>
          {shown.translation}
        </span>
      )}
    </>
  );
}
