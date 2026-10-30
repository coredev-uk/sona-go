import { UpdatesService } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { ArrowUpCircle } from "lucide-react";

import { keys } from "@/shared/lib/keys";
import { run } from "@/shared/lib/run";

/**
 * The quiet update line at the foot of the sidebar, shown only while a
 * newer release than this build exists: its version and a link to it.
 */
export function UpdateNotice() {
  const st = useQuery({
    queryKey: keys.updates,
    queryFn: () => UpdatesService.Status(),
    staleTime: Infinity,
  }).data;
  const { t } = useLingui();
  if (!st?.available) return null;
  const latest = st.latest;
  return (
    <div className="mb-2 flex items-center justify-between gap-2 px-2.5 text-meta" role="status">
      <span className="flex min-w-0 items-center gap-1.5 text-ink">
        <ArrowUpCircle aria-hidden className="size-3 shrink-0" strokeWidth={2} />
        <span className="truncate">
          <Trans>Sona {latest} is available</Trans>
        </span>
      </span>
      <button
        type="button"
        onClick={() => void run(t`Couldn't open the release`, UpdatesService.OpenRelease())}
        className="shrink-0 rounded-sm text-ink-muted underline-offset-2 hover:text-ink hover:underline"
      >
        <Trans>View</Trans>
      </button>
    </div>
  );
}
