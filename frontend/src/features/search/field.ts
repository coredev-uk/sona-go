/** The top bar's search input while it is mounted, for the focus shortcut. */
let field: HTMLInputElement | null = null;

export function bindSearchField(el: HTMLInputElement | null) {
  field = el;
}

/** Focuses the top bar's search field and selects its text. */
export function focusSearchField() {
  field?.focus();
  field?.select();
}
