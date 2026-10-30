import { Trans, useLingui } from "@lingui/react/macro";
import { Link, useRouter } from "@tanstack/react-router";
import { ChevronLeft, ChevronRight, User } from "lucide-react";

import { Avatar, AvatarFallback } from "@/shared/ui/avatar";
import { Button } from "@/shared/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import { slots } from "../compose";

/** The search field a module provides (the search feature's). */
const SearchSlot = slots.find((s) => s.TopBarSearch)?.TopBarSearch;

export function TopBar() {
  const { t } = useLingui();
  const router = useRouter();
  return (
    <header className="flex h-12 items-center gap-3 bg-surface pr-4 pl-3">
      <div className="flex items-center gap-0.5">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t`Back`}
              onClick={() => router.history.back()}
            >
              <ChevronLeft strokeWidth={1.75} />
            </Button>
          </TooltipTrigger>
          <TooltipContent>
            <Trans>Back</Trans>
          </TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t`Forward`}
              onClick={() => router.history.forward()}
            >
              <ChevronRight strokeWidth={1.75} />
            </Button>
          </TooltipTrigger>
          <TooltipContent>
            <Trans>Forward</Trans>
          </TooltipContent>
        </Tooltip>
      </div>
      {SearchSlot && <SearchSlot />}
      <Tooltip>
        <TooltipTrigger asChild>
          <Link
            to="/settings"
            search={{ section: "account" }}
            aria-label={t`Account`}
            className="ml-auto rounded-full"
          >
            <Avatar className="size-7">
              <AvatarFallback className="bg-hover text-ink-muted">
                <User aria-hidden className="size-4" strokeWidth={1.75} />
              </AvatarFallback>
            </Avatar>
          </Link>
        </TooltipTrigger>
        <TooltipContent>
          <Trans>Account</Trans>
        </TooltipContent>
      </Tooltip>
    </header>
  );
}
