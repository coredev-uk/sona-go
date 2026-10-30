/** The marker Radix's `hideOthers` sets on what it hides behind a modal. */
const MARKER = "data-aria-hidden";

/**
 * Makes everything a modal layer (dialog, menu, select) hides from
 * assistive technology inert as well. Radix only sets `aria-hidden` on the
 * rest of the app, which leaves the hidden links and buttons focusable
 * (axe `aria-hidden-focus`, serious): `inert` removes them from the tab
 * order and the accessibility tree together, and goes when the marker
 * does, before Radix returns focus to the trigger. Elements already inert
 * for another reason (the page under the Now Playing view) stay inert.
 */
export function mirrorModalInert(root: Node = document.body): () => void {
  const ours = new WeakSet<Element>();
  const sync = (el: Element) => {
    if (el.getAttribute(MARKER) === "true") {
      if (el.hasAttribute("inert")) return;
      el.setAttribute("inert", "");
      ours.add(el);
    } else if (ours.delete(el)) {
      el.removeAttribute("inert");
    }
  };
  const observer = new MutationObserver((records) => {
    for (const r of records) if (r.target instanceof Element) sync(r.target);
  });
  observer.observe(root, { subtree: true, attributes: true, attributeFilter: [MARKER] });
  return () => observer.disconnect();
}
