/** What the palette ranks: a label and optional extra words. */
export interface Rankable {
  label: () => string;
  keywords?: () => string;
}

/** Lower case, accents folded, spaces collapsed: "Café  Mix" → "cafe mix". */
function fold(text: string): string {
  return text.normalize("NFD").replace(/\p{M}/gu, "").toLowerCase().replace(/\s+/g, " ").trim();
}

const words = (text: string) => text.split(/[^\p{L}\p{N}]+/u).filter(Boolean);

/** Whether `q`'s letters appear in `text` in order. */
function isSubsequence(q: string, text: string): boolean {
  let i = 0;
  for (const c of text) if (c === q[i]) i++;
  return i === q.length;
}

/**
 * How well `query` matches an item, higher is better, 0 for no match:
 * the whole label, then its start, a word's start, every typed word
 * starting a label word, anywhere in the label, every typed word starting
 * a label or keyword word, and last the letters in order ("tgq").
 */
export function matchScore(query: string, item: Rankable): number {
  const q = fold(query);
  if (!q) return 1;
  const label = fold(item.label());
  if (label === q) return 100;
  if (label.startsWith(q)) return 90;
  const labelWords = words(label);
  if (labelWords.some((w) => w.startsWith(q))) return 80;
  const typed = words(q);
  const startsWord = (pool: string[]) => typed.every((t) => pool.some((w) => w.startsWith(t)));
  if (typed.length && startsWord(labelWords)) return 70;
  if (label.includes(q)) return 60;
  if (typed.length && startsWord([...labelWords, ...words(fold(item.keywords?.() ?? ""))]))
    return 40;
  if (isSubsequence(q.replace(/ /g, ""), label)) return 20;
  return 0;
}

/** The items matching `query`, best first; equal scores keep their given order. */
export function rankCommands<T extends Rankable>(query: string, items: readonly T[]): T[] {
  return items
    .map((item, i) => ({ item, i, score: matchScore(query, item) }))
    .filter((r) => r.score > 0)
    .sort((a, b) => b.score - a.score || a.i - b.i)
    .map((r) => r.item);
}
