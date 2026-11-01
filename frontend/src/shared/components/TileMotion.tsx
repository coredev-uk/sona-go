import { useEffect, useId } from "react";

import { MotionVideo } from "@/shared/components/MotionCover";
import { listenForTileMotion, motionAt, useMotionState, useTileMotion } from "@/shared/lib/motion";

/** Tiles are requested at this CSS width: the 360 px rendition at 2×. */
const TILE_VIDEO_CSS = 180;

/**
 * A cover tile's motion artwork, played only while its tile (the
 * `data-cover-tile` ancestor) is hovered or focused, after a short dwell,
 * and only one tile at a time (shared/lib/motion's delegated listeners).
 * At rest it is a hidden marker: no video, no observer, no layer. Render it
 * inside the tile's positioned art box.
 */
export function TileMotion({ motion, round }: { motion: string; round?: boolean }) {
  const id = useId();
  const active = useTileMotion((s) => s.active === id);
  const { allowed, codec } = useMotionState();
  useEffect(() => {
    if (allowed) listenForTileMotion();
  }, [allowed]);
  if (!allowed) return null;
  return (
    <>
      <span hidden data-tile-motion={id} />
      {active && <MotionVideo src={motionAt(motion, TILE_VIDEO_CSS, codec)} round={round} />}
    </>
  );
}
