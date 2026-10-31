import { Trans, useLingui } from "@lingui/react/macro";
import { Link, useRouter } from "@tanstack/react-router";
import { ChevronLeft, ChevronRight, User } from "lucide-react";

import { Avatar, AvatarFallback } from "@/shared/ui/avatar";
import { Button } from "@/shared/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import { slots } from "../compose";
import { useLayout } from "./layout";
import { titleBarRegion, useWindowChrome } from "./window-chrome";
import { WindowControls } from "./WindowControls";

/** The search field a module provides (the search feature's). */
const SearchSlot = slots.find((s) => s.TopBarSearch)?.TopBarSearch;

/**
 * The top bar, which is also the window's title bar where Sona draws it
 * (window-chrome.ts): history, search, the account, and the window's own
 * buttons at the desktop's side. While the sidebar is hidden it also takes
 * the leading end (macOS's buttons, Sona's leading ones).
 */
export function TopBar() {
  const { t } = useLingui();
  const router = useRouter();
  const chrome = useWindowChrome();
  const sidebar = useLayout((s) => s.sidebar);
  const region = titleBarRegion(chrome);
  const leading = !sidebar && chrome.start.length > 0;
  const trailing = chrome.end.length > 0;
  let paddingStart = 12;
  if (!sidebar && chrome.startRoom) paddingStart = chrome.startRoom;
  else if (leading) paddingStart = 8;
  let paddingEnd = 16;
  if (trailing) paddingEnd = chrome.captions ? 0 : 8;
  return (
    <header
      {...region}
      style={{
        ...region.style,
        height: chrome.height,
        paddingInlineStart: paddingStart,
        paddingInlineEnd: paddingEnd,
      }}
      className="flex shrink-0 items-center gap-3 bg-surface select-none"
    >
      {leading && <WindowControls chrome={chrome} side="start" />}
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
      {SearchSlot && (
        <div className="flex min-w-60 flex-[0_1_520px]">
          <SearchSlot />
        </div>
      )}
      {/* Room to grab the window by, at every width. */}
      <div aria-hidden className="h-full min-w-16 flex-1" />
      <Tooltip>
        <TooltipTrigger asChild>
          <Link
            to="/settings"
            search={{ section: "account" }}
            aria-label={t`Account`}
            className="rounded-full"
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
      {trailing && <WindowControls chrome={chrome} side="end" />}
    </header>
  );
}
