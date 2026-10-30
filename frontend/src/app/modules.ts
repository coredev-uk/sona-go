import { albumModule } from "@/features/album";
import { artistModule } from "@/features/artist";
import { authModule } from "@/features/auth";
import { discoverModule } from "@/features/discover";
import { homeModule } from "@/features/home";
import { libraryModule } from "@/features/library";
import { playerModule } from "@/features/player";
import { playlistModule } from "@/features/playlist";
import { radioModule } from "@/features/radio";
import { searchModule } from "@/features/search";
import { songModule } from "@/features/song";
import { videoModule } from "@/features/video";
import { settingsModule } from "@/features/settings";

/**
 * Every feature, in registration order: routes, event handlers, shortcuts
 * and shell slots follow it (nav items and actions sort by their own
 * `order`). Adding a feature is one line here.
 */
export const modules = [
  homeModule,
  discoverModule,
  searchModule,
  libraryModule,
  albumModule,
  artistModule,
  songModule,
  videoModule,
  playlistModule,
  radioModule,
  playerModule,
  settingsModule,
  authModule,
] as const;
