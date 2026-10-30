import { z } from "zod";

/** /album/$id?song=… marks and scrolls to one track (shared song links). */
export const albumSearch = z.object({
  song: z.string().optional().catch(undefined),
});
