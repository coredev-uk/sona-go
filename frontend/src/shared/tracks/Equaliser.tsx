import { useLingui } from "@lingui/react/macro";

import { useNowPlaying } from "@/shared/lib/now-playing";
import { cn } from "@/shared/lib/utils";

/**
 * The now-playing mark: three amber bars, moving while the track plays and
 * still when paused, and still while the Now Playing view or full-screen
 * lyrics cover it (an unseen loop still costs every frame). It replaces a
 * row's index wherever the playing track appears.
 */
export function Equaliser({ playing, className }: { playing: boolean; className?: string }) {
  const { covered } = useNowPlaying();
  const { t } = useLingui();
  return (
    <span
      role="img"
      aria-label={playing ? t`Now playing` : t`Paused`}
      className={cn(
        "inline-flex h-3 items-end gap-[2px]",
        playing && !covered && "eq-live",
        className,
      )}
    >
      <span className="eq-bar h-[60%]" />
      <span className="eq-bar h-full" />
      <span className="eq-bar h-[40%]" />
    </span>
  );
}
