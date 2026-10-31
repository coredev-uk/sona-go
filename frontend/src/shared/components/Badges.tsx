import type { Traits } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";

import { traitLabels } from "@/shared/lib/traits";
import { cn } from "@/shared/lib/utils";

/** The small "E" beside explicit titles in rows and cards. */
export function ExplicitMark({ className }: { className?: string }) {
  const { t } = useLingui();
  return (
    <span
      role="img"
      aria-label={t`Explicit`}
      title={t`Explicit`}
      className={cn(
        "grid size-3.5 shrink-0 place-items-center rounded-[3px] bg-ink-faint/25 text-[9px] leading-none font-semibold text-ink-muted",
        className,
      )}
    >
      <Trans context="explicit content mark">E</Trans>
    </span>
  );
}

function Tag({ children, title }: { children: string; title?: string }) {
  return (
    <span
      title={title}
      className="inline-flex h-5 flex-none items-center rounded-[4px] border border-control-line px-1.5 text-[11px] leading-none font-medium whitespace-nowrap text-ink-muted"
    >
      {children}
    </span>
  );
}

/**
 * "Preview" (Apple's word for it) beside a song that plays as Apple Music's
 * 30-second preview rather than the full track.
 */
export function PreviewMark() {
  const { t } = useLingui();
  return (
    <Tag title={t`Apple Music's 30-second preview, not the full song`}>
      {t({ message: "Preview", context: "a song playing as its 30-second preview" })}
    </Tag>
  );
}

/**
 * The badge line of a collection or song header: Explicit, then Apple's
 * audio labels (Dolby Atmos, Lossless, Apple Digital Master). The labels
 * describe the release as Apple offers it, not what Sona plays (AAC).
 * Renders nothing when the item has none.
 */
export function Badges({
  explicit,
  traits,
  extra,
  className,
}: {
  explicit?: boolean;
  traits?: Traits;
  /** Further labels after Apple's audio ones (a video's "4K", "HDR"). */
  extra?: readonly string[];
  className?: string;
}) {
  const { t } = useLingui();
  const audio = traitLabels(traits);
  if (!explicit && audio.length === 0 && !extra?.length) return null;
  return (
    <span className={cn("inline-flex flex-wrap items-center gap-1.5", className)}>
      {explicit && <Tag>{t`Explicit`}</Tag>}
      {audio.map((label) => (
        <Tag key={label} title={t`${label}: Apple's label for this release. Sona plays it as AAC.`}>
          {label}
        </Tag>
      ))}
      {extra?.map((l) => (
        <Tag key={l}>{l}</Tag>
      ))}
    </span>
  );
}
