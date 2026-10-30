import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { Dialog as DialogPrimitive } from "radix-ui";
import { X } from "lucide-react";
import { useRef } from "react";

import { shortcutGroups } from "@/shared/module";
import { useShortcutBindings } from "@/shared/shortcuts/bindings";
import { groupTitle } from "@/shared/shortcuts/groups";
import { eventChord } from "@/shared/shortcuts/chord";
import { BindingKeys } from "@/shared/shortcuts/Keys";
import { registeredShortcuts } from "@/shared/shortcuts/registry";
import { DialogDescription, DialogOverlay, DialogPortal, DialogTitle } from "@/shared/ui/dialog";

import { useCommands } from "./store";

/**
 * "?": every shortcut by group with its current keys, generated from the
 * registry, so a rebinding or a new module's shortcut shows up here at once.
 */
export function ShortcutHelp() {
  const open = useCommands((s) => s.open === "help");
  return (
    <DialogPrimitive.Root
      open={open}
      onOpenChange={(o) => !o && useCommands.getState().show("none")}
    >
      {open && <HelpBody />}
    </DialogPrimitive.Root>
  );
}

function HelpBody() {
  const { t } = useLingui();
  const bindings = useShortcutBindings();
  const all = registeredShortcuts();
  const close = () => useCommands.getState().show("none");
  const listRef = useRef<HTMLDivElement>(null);

  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPrimitive.Content
        onOpenAutoFocus={(e) => {
          // The list takes focus, so arrows and Page Down scroll it.
          e.preventDefault();
          listRef.current?.focus();
        }}
        onCloseAutoFocus={(e) => {
          e.preventDefault();
          if (useCommands.getState().open === "none") useCommands.getState().restoreFocus();
        }}
        onKeyDown={(e) => {
          // The sheet's own key closes it again.
          const chord = eventChord(e.nativeEvent);
          if (chord && bindings.get("commands.help")?.includes(chord)) {
            e.preventDefault();
            close();
          }
        }}
        className="fixed top-1/2 left-1/2 z-50 flex max-h-[min(80vh,720px)] w-[min(760px,calc(100vw-2rem))] -translate-x-1/2 -translate-y-1/2 flex-col rounded-lg border border-line bg-raised shadow-float duration-150 outline-none data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:slide-in-from-bottom-1"
      >
        <header className="flex shrink-0 items-start justify-between gap-6 px-6 pt-5 pb-4">
          <div>
            <DialogTitle className="text-heading text-ink">
              <Trans>Keyboard shortcuts</Trans>
            </DialogTitle>
            <DialogDescription className="mt-0.5 text-sub text-ink-muted">
              <Trans>
                Change any of them in{" "}
                <Link
                  to="/settings"
                  search={{ section: "shortcuts" }}
                  onClick={close}
                  className="text-ink underline underline-offset-2"
                >
                  Settings
                </Link>
                .
              </Trans>
            </DialogDescription>
          </div>
          <DialogPrimitive.Close
            aria-label={t`Close`}
            className="-mr-2 grid size-7 shrink-0 place-items-center rounded-md text-ink-muted transition-colors duration-100 hover:bg-hover hover:text-ink"
          >
            <X aria-hidden className="size-4" strokeWidth={1.75} />
          </DialogPrimitive.Close>
        </header>
        <div
          ref={listRef}
          tabIndex={0}
          role="region"
          aria-label={t`Shortcuts`}
          className="min-h-0 overflow-y-auto rounded-b-lg px-6 pb-6 focus-visible:outline-offset-[-2px]"
        >
          <div className="columns-2 gap-10 max-[720px]:columns-1">
            {shortcutGroups.map((group) => {
              const list = all.filter((s) => s.group === group);
              if (!list.length) return null;
              return (
                <section key={group} className="mb-6 break-inside-avoid">
                  <h3 className="mb-1.5 text-ui font-semibold text-ink">{groupTitle(group)}</h3>
                  <dl>
                    {list.map((s) => {
                      const chords = bindings.get(s.id) ?? [];
                      return (
                        <div
                          key={s.id}
                          className="flex min-h-8 items-center justify-between gap-4 py-0.5"
                        >
                          <dt className="min-w-0 text-ui text-ink">{s.label()}</dt>
                          <dd className="shrink-0">
                            {chords.length ? (
                              <BindingKeys chords={chords} />
                            ) : (
                              <span className="text-meta text-ink-faint">
                                <Trans>Not set</Trans>
                              </span>
                            )}
                          </dd>
                        </div>
                      );
                    })}
                  </dl>
                </section>
              );
            })}
          </div>
        </div>
      </DialogPrimitive.Content>
    </DialogPortal>
  );
}
