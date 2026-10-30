import { useLingui } from "@lingui/react/macro";
import { useEffect, useState } from "react";

import { announcedTrack } from "./announce";
import { usePlayback } from "./playback";

/**
 * Announces a new track to screen readers, politely (role="status"), when
 * it starts: track changes only, never play/pause or seeking (announcedTrack).
 * Each announcement is a new paragraph (keyed by track): WebKitGTK sends
 * AT-SPI no event when text inside a live region changes, only when a
 * child is added, so a reused text node would never be spoken. The player
 * bar and the mini player each hold one: only the shown window is read.
 */
export function TrackAnnouncer() {
  const [track, setTrack] = useState<{ id: string; name: string; artist: string } | null>(null);
  const { t } = useLingui();
  useEffect(() => {
    let seen: string | undefined;
    return usePlayback.subscribe((s) => {
      const c = announcedTrack(seen, s);
      if (!c) return;
      seen = c.id;
      setTrack({ id: c.id, name: c.name, artist: c.artistName });
    });
  }, []);
  const name = track?.name ?? "";
  const artist = track?.artist ?? "";
  return (
    <div role="status" className="sr-only">
      {track && (
        <p key={track.id}>
          {artist ? t`Now playing: ${name} by ${artist}` : t`Now playing: ${name}`}
        </p>
      )}
    </div>
  );
}
