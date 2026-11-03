import { avatarTone, initialsOf } from "@/shared/lib/avatar";

import { Artwork } from "./Artwork";

interface PersonAvatarProps {
  /** The person's name, shown beside the avatar by the caller. */
  name: string;
  /** Go-served picture; without one (or while it fails) the initials are drawn. */
  src?: string;
  /** Rendered diameter in CSS pixels. */
  size: number;
  className?: string;
}

/**
 * A person's picture, or their initials on a tint picked from their name.
 *
 * Decorative: the name is always written next to it, so it is hidden from
 * assistive technology rather than read twice.
 */
export function PersonAvatar({ name, src, size, className }: PersonAvatarProps) {
  const tone = avatarTone(name);
  return (
    <span aria-hidden="true" className="contents">
      <Artwork
        src={src}
        size={size}
        alt=""
        shape="round"
        className={className}
        fallback={
          <span
            className="absolute inset-0 grid place-items-center rounded-full leading-none font-semibold outline -outline-offset-1 outline-ink/6 select-none"
            style={{
              background: `var(--avatar-${tone}-bg)`,
              color: `var(--avatar-${tone}-fg)`,
              fontSize: Math.round(size * 0.38),
            }}
          >
            {initialsOf(name)}
          </span>
        }
      />
    </span>
  );
}
