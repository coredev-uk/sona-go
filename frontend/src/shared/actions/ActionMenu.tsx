import { useNavigate } from "@tanstack/react-router";
import { Ellipsis } from "lucide-react";
import { Fragment, useState } from "react";

import { Button } from "@/shared/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu";

import {
  actionsFor,
  menuSections,
  type ActionContainer,
  type ActionContext,
  type ActionTargetKind,
  type ActionTargets,
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
}

/**
 * A row's "more" menu: every registered action for the target, grouped and
 * ordered by the registry. Menu data (ratings) is read when the menu opens,
 * not for every row.
 */
export function ActionMenu<K extends ActionTargetKind>({
  kind,
  target,
  name,
  variant = "row",
  container,
}: ActionMenuProps<K>) {
  const navigate = useNavigate();
  const [loaded, setLoaded] = useState<Record<string, unknown>>({});
  const ctx: ActionContext<K> = {
    navigate,
    data<V>(d: MenuData<K, V>) {
      return loaded[d.id] as V | undefined;
    },
    container,
  };
  const actions = actionsFor(kind).filter((a) => a.isAvailable?.(target, ctx) ?? true);

  const onOpen = (open: boolean) => {
    if (!open) return;
    const pending = new Map(actions.flatMap((a) => a.data ?? []).map((d) => [d.id, d]));
    for (const d of pending.values()) {
      void d.load(target).then((value) => setLoaded((prev) => ({ ...prev, [d.id]: value })));
    }
  };

  return (
    <DropdownMenu onOpenChange={onOpen}>
      <DropdownMenuTrigger asChild>
        {variant === "header" ? (
          <Button variant="outline" size="icon-sm" aria-label={`More for ${name}`}>
            <Ellipsis />
          </Button>
        ) : (
          <Button
            variant="ghost"
            size="icon-xs"
            tabIndex={-1}
            aria-label={`More for ${name}`}
            className="text-ink-muted opacity-0 group-focus-within/row:opacity-100 group-hover/row:opacity-100 hover:text-ink data-[state=open]:opacity-100"
          >
            <Ellipsis />
          </Button>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-52">
        {menuSections(actions).map((section, i) => (
          <Fragment key={section.group}>
            {i > 0 && <DropdownMenuSeparator />}
            {section.actions.map((a) => {
              const Icon = a.icon;
              return (
                <DropdownMenuItem
                  key={a.id}
                  disabled={a.isEnabled ? !a.isEnabled(target, ctx) : undefined}
                  onSelect={() => a.run(target, ctx)}
                >
                  {a.filled ? (
                    <Icon fill={a.filled(target, ctx) ? "currentColor" : "none"} />
                  ) : (
                    <Icon />
                  )}
                  {typeof a.label === "string" ? a.label : a.label(target, ctx)}
                </DropdownMenuItem>
              );
            })}
          </Fragment>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
