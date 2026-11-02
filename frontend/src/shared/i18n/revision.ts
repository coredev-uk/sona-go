/**
 * Counts catalogue changes that keep the locale: Apple's translations of
 * the shown locale updating while Sona runs. `I18nRoot` keys its subtree
 * on it, as on the locale, so strings computed outside hooks follow.
 */
let revision = 0;
const listeners = new Set<() => void>();

export function bumpRevision(): void {
  revision++;
  listeners.forEach((l) => l());
}

export function subscribeRevision(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function getRevision(): number {
  return revision;
}
