import { createRoute } from "@tanstack/react-router";

import type { FeatureModule } from "@/shared/module";
import { childRoute } from "@/shared/router";

import { VideoPage } from "./VideoPage";

/** A catalogue music video's page; `id` is a catalogue music video ID. */
const videoRoute = createRoute({ ...childRoute("/music-video/$id"), component: VideoPage });

/**
 * The music video page: plays the video in the player's video view where
 * videos play, and opens it in Apple Music on the web.
 */
export const videoModule: FeatureModule<typeof videoRoute> = {
  id: "video",
  routes: [videoRoute],
};
