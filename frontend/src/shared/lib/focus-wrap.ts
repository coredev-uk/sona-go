/**
 * Keeps Tab inside the page: Tab on the last focusable element goes to the
 * first, Shift+Tab on the first goes to the last. WebKitGTK hands focus to
 * GTK when Tab runs off either end of the document, and GTK gives it to
 * the hidden text widget WebKit keeps beside the page, so no key reached
 * the page again (a keyboard trap, docs/accessibility.md). Tab that a
 * focus trap (dialogs) already handled is left alone.
 */
export function wrapTabFocus(doc: Document = document): () => void {
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key !== "Tab" || e.defaultPrevented || e.ctrlKey || e.altKey || e.metaKey) return;
    const target = edgeTarget(doc, e.shiftKey);
    if (!target) return;
    e.preventDefault();
    target.focus();
  };
  doc.defaultView?.addEventListener("keydown", onKeyDown);
  return () => doc.defaultView?.removeEventListener("keydown", onKeyDown);
}

/** Where Tab (or Shift+Tab with `back`) wraps to, or null when it does not wrap. */
function edgeTarget(doc: Document, back: boolean): HTMLElement | null {
  const all = tabbables(doc);
  if (all.length === 0) return null;
  const first = all[0];
  const last = all[all.length - 1];
  const active = doc.activeElement;
  if (!active || active === doc.body || active === doc.documentElement) {
    return back ? last : null;
  }
  if (back) {
    const precedes = first.compareDocumentPosition(active) & Node.DOCUMENT_POSITION_FOLLOWING;
    return active === first || !precedes ? last : null;
  }
  const follows = active.compareDocumentPosition(last) & Node.DOCUMENT_POSITION_FOLLOWING;
  return active === last || !follows ? first : null;
}

/** Elements Tab can reach, in document order. */
function tabbables(doc: Document): HTMLElement[] {
  const out: HTMLElement[] = [];
  const walker = doc.createTreeWalker(doc.body, NodeFilter.SHOW_ELEMENT, {
    acceptNode(node) {
      const el = node as HTMLElement;
      if (el.hasAttribute("inert") || el.hidden) return NodeFilter.FILTER_REJECT;
      const style = doc.defaultView?.getComputedStyle(el);
      if (style?.display === "none") return NodeFilter.FILTER_REJECT;
      if (el.tabIndex < 0 || (el as HTMLButtonElement).disabled) return NodeFilter.FILTER_SKIP;
      if (el.tagName === "INPUT" && (el as HTMLInputElement).type === "hidden") {
        return NodeFilter.FILTER_SKIP;
      }
      if (el.tagName === "A" && !el.hasAttribute("href") && !el.hasAttribute("tabindex")) {
        return NodeFilter.FILTER_SKIP;
      }
      if (style?.visibility === "hidden") return NodeFilter.FILTER_SKIP;
      return NodeFilter.FILTER_ACCEPT;
    },
  });
  for (let n = walker.nextNode(); n; n = walker.nextNode()) out.push(n as HTMLElement);
  return out;
}
