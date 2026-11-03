import { Trans, useLingui } from "@lingui/react/macro";
import { useNavigate } from "@tanstack/react-router";
import { Ellipsis, type LucideIcon } from "lucide-react";
import { Fragment, useContext, useEffect, useEffectEvent, useState, type ReactNode } from "react";

import { SelectionContext } from "@/shared/selection/store";
import { Button } from "@/shared/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu";

import {
  actionsFor,
  bulkActionsFor,
  describeItems,
  menuSections,
  type ActionContainer,
  type ActionContext,
  type ActionGroup,
  type ActionItem,
  type ActionTargetKind,
  type ActionTargets,
  type BulkActionContext,
  type BulkMenuData,
  type MenuData,
} from "./registry";

interface ActionMenuProps<K extends ActionTargetKind> {
  kind: K;
  target: ActionTargets[K];
  /** The item's name, for the trigger's accessible name: "More for Airbag". */
  name: string;
  /**
   * "row" (default): a quiet trigger shown while its row is hovered or
   * focused. "header": an always-visible outline button for a collection
   * header's action row.
   */
  variant?: "row" | "header";
  /** The collection the item is listed in (see ActionContainer). */
  container?: ActionContainer;
  /**
   * The item's key in the enclosing list's selection: when the item is one
   * of several selected, the menu acts on all of them (the targets[] form).
   */
  selectionKey?: string;
}

const rowTriggerClassName =
  "text-ink-muted opacity-0 group-focus-within/row:opacity-100 group-hover/row:opacity-100 hover:text-ink data-[state=open]:opacity-100";

/**
 * A row's "more" menu: every registered action for the target, grouped and
 * ordered by the registry. Menu data (ratings) is read when the menu opens,
 * not for every row. When the row is part of a multi-selection the menu
 * lists the bulk actions for the whole selection instead.
 *
 * A row's trigger starts as a plain button: the Radix menu (its providers
 * and document listeners) mounts when the pointer first reaches it, so long
 * lists and grids pay for one button per row. A press that arrives before
 * then opens the menu as it mounts.
 */
export function ActionMenu<K extends ActionTargetKind>(props: ActionMenuProps<K>) {
  const { t } = useLingui();
  const [armed, setArmed] = useState(props.variant === "header");
  const [open, setOpen] = useState(false);
  // Opened by Enter/Space or an assistive technology's click (no pointer).
  const [keyboard, setKeyboard] = useState(false);
  const name = props.name;
  if (armed) return <Menu {...props} open={open} onOpenChange={setOpen} keyboard={keyboard} />;
  return (
    <Button
      variant="ghost"
      size="icon-xs"
      tabIndex={-1}
      aria-label={t`More for ${name}`}
      aria-haspopup="menu"
      className={rowTriggerClassName}
      onPointerEnter={() => setArmed(true)}
      onPointerDown={(e) => {
        if (e.button !== 0 || e.ctrlKey) return;
        e.preventDefault();
        setArmed(true);
        setOpen(true);
      }}
      onClick={(e) => {
        setKeyboard(e.detail === 0);
        setArmed(true);
        setOpen(true);
      }}
    >
      <Ellipsis />
    </Button>
  );
}

/** ActionMenu for an item of any kind (cover tiles hold an ActionItem). */
export function ItemActionMenu({
  item,
  ...rest
}: { item: ActionItem } & Omit<ActionMenuProps<ActionTargetKind>, "kind" | "target">) {
  return <ActionMenu {...rest} kind={item.kind} target={item.target} />;
}

/**
 * The same menu opened at a point: right-click, or Shift+F10 / the Menu key
 * on a focused row. It mounts open and unmounts through `onClose`; focus
 * returns to `returnFocus` (the row), not to the invisible anchor.
 */
export function ContextActionMenu({
  item,
  at,
  onClose,
  returnFocus,
  keyboard,
  ...rest
}: {
  item: ActionItem;
  at: { x: number; y: number };
  onClose: () => void;
  returnFocus?: () => HTMLElement | null | undefined;
  /** Opened with Shift+F10 or the Menu key: focus starts on the first item. */
  keyboard?: boolean;
} & Omit<ActionMenuProps<ActionTargetKind>, "kind" | "target" | "variant">) {
  return (
    <Menu
      {...rest}
      kind={item.kind}
      target={item.target}
      open
      onOpenChange={(o) => !o && onClose()}
      anchor={at}
      returnFocus={returnFocus}
      keyboard={keyboard}
    />
  );
}

function Menu<K extends ActionTargetKind>({
  kind,
  target,
  name,
  variant = "row",
  container,
  selectionKey,
  open,
  onOpenChange,
  anchor,
  returnFocus,
  keyboard,
}: ActionMenuProps<K> & {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  anchor?: { x: number; y: number };
  returnFocus?: () => HTMLElement | null | undefined;
  keyboard?: boolean;
}) {
  const { t } = useLingui();
  const scope = useContext(SelectionContext);
  const groupNow = () => (scope && selectionKey ? scope.groupWith(selectionKey) : null);
  // The selection the menu acts on is taken when it opens.
  const [group, setGroup] = useState(() => (open ? groupNow() : null));
  const changeOpen = (o: boolean) => {
    if (o) setGroup(groupNow());
    onOpenChange(o);
  };

  let trigger: ReactNode;
  if (anchor) {
    trigger = (
      <span
        aria-hidden
        className="pointer-events-none fixed size-0"
        style={{ left: anchor.x, top: anchor.y }}
      />
    );
  } else if (variant === "header") {
    trigger = (
      <Button variant="outline" size="icon-sm" aria-label={t`More for ${name}`}>
        <Ellipsis />
      </Button>
    );
  } else {
    trigger = (
      <Button
        variant="ghost"
        size="icon-xs"
        tabIndex={-1}
        aria-label={t`More for ${name}`}
        className={rowTriggerClassName}
      >
        <Ellipsis />
      </Button>
    );
  }

  return (
    <DropdownMenu open={open} onOpenChange={changeOpen}>
      <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
      <DropdownMenuContent
        align={anchor ? "start" : "end"}
        sideOffset={anchor ? 2 : undefined}
        className="w-56"
        // Radix focuses the content itself when it mounts open; a keyboard
        // open starts on the first item instead, after that.
        ref={
          keyboard
            ? (el: HTMLDivElement | null) => {
                if (!el) return;
                requestAnimationFrame(() =>
                  el.querySelector<HTMLElement>("[role^=menuitem]:not([data-disabled])")?.focus(),
                );
              }
            : undefined
        }
        onCloseAutoFocus={
          returnFocus
            ? (e) => {
                const el = returnFocus();
                if (!el) return;
                e.preventDefault();
                el.focus({ preventScroll: true });
              }
            : undefined
        }
      >
        {group ? (
          <BulkItems
            items={group.items}
            indices={group.indices}
            container={container}
            open={open}
          />
        ) : (
          <SingleItems kind={kind} target={target} container={container} open={open} />
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** One menu row: an icon (filled when `filled`) and a label. */
function Row({
  icon: Icon,
  label,
  filled,
  disabled,
  reason,
  onSelect,
}: {
  icon: LucideIcon;
  label: string;
  filled?: boolean;
  disabled?: boolean;
  /** Why the row is greyed out, said under its label. */
  reason?: string;
  onSelect: () => void;
}) {
  return (
    <DropdownMenuItem disabled={disabled || !!reason} onSelect={onSelect}>
      {filled === undefined ? <Icon /> : <Icon fill={filled ? "currentColor" : "none"} />}
      {reason ? (
        <span className="flex min-w-0 flex-col">
          {label}
          <span className="text-meta font-normal text-ink-faint">{reason}</span>
        </span>
      ) : (
        label
      )}
    </DropdownMenuItem>
  );
}

function Sections<A extends { id: string; group: ActionGroup }>({
  actions,
  row,
}: {
  actions: readonly A[];
  row: (a: A) => ReactNode;
}) {
  return menuSections(actions).map((section, i) => (
    <Fragment key={section.group}>
      {i > 0 && <DropdownMenuSeparator />}
      {section.actions.map(row)}
    </Fragment>
  ));
}

/** Loads every distinct menu data an action list reads, once the menu is open. */
function useMenuData<T>(
  open: boolean,
  datas: readonly { id: string; load: (arg: T) => Promise<unknown> }[],
  arg: T,
) {
  const [loaded, setLoaded] = useState<Record<string, unknown>>({});
  const load = useEffectEvent(() => {
    const pending = new Map(datas.map((d) => [d.id, d]));
    for (const d of pending.values()) {
      void d.load(arg).then((value) => setLoaded((prev) => ({ ...prev, [d.id]: value })));
    }
  });
  useEffect(() => {
    if (open) load();
  }, [open]);
  return loaded;
}

function SingleItems<K extends ActionTargetKind>({
  kind,
  target,
  container,
  open,
}: {
  kind: K;
  target: ActionTargets[K];
  container?: ActionContainer;
  open: boolean;
}) {
  const navigate = useNavigate();
  const all = actionsFor(kind);
  const loaded = useMenuData(
    open,
    all.flatMap((a) => a.data ?? []),
    target,
  );
  const ctx: ActionContext<K> = {
    navigate,
    data<V>(d: MenuData<K, V>) {
      return loaded[d.id] as V | undefined;
    },
    container,
  };
  const actions = all.filter((a) => a.isAvailable?.(target, ctx) ?? true);
  return (
    <Sections
      actions={actions}
      row={(a) => (
        <Row
          key={a.id}
          icon={a.icon}
          label={a.label(target, ctx)}
          filled={a.filled?.(target, ctx)}
          disabled={a.isEnabled ? !a.isEnabled(target, ctx) : undefined}
          reason={a.disabledReason?.(target, ctx)}
          onSelect={() => a.run(target, ctx)}
        />
      )}
    />
  );
}

function BulkItems({
  items,
  indices,
  container,
  open,
}: {
  items: readonly ActionItem[];
  indices: readonly number[];
  container?: ActionContainer;
  open: boolean;
}) {
  const navigate = useNavigate();
  const all = bulkActionsFor(items);
  const loaded = useMenuData(
    open,
    all.flatMap((a) => a.data ?? []),
    items,
  );
  const ctx: BulkActionContext = {
    navigate,
    data<V>(d: BulkMenuData<V>) {
      return loaded[d.id] as V | undefined;
    },
    container: container && {
      kind: container.kind,
      id: container.id,
      editable: container.editable,
      indices,
    },
  };
  const actions = all.filter((a) => a.isAvailable?.(items, ctx) ?? true);
  const what = describeItems(items);
  return (
    <>
      <DropdownMenuLabel className="text-meta font-normal text-ink-faint num">
        <Trans>{what} selected</Trans>
      </DropdownMenuLabel>
      <Sections
        actions={actions}
        row={(a) => (
          <Row
            key={a.id}
            icon={a.icon}
            label={a.label(items, ctx)}
            filled={a.filled?.(items, ctx)}
            disabled={a.isEnabled ? !a.isEnabled(items, ctx) : undefined}
            reason={a.disabledReason?.(items, ctx)}
            onSelect={() => a.run(items, ctx)}
          />
        )}
      />
    </>
  );
}
