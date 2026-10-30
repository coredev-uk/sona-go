import { z } from "zod";

/** /room/$id?sort=… : one of the room's sorts ("" is Apple's default). */
export const roomSearch = z.object({
  sort: z.string().optional().catch(undefined),
});

/** /charts?genre=… : a chart genre ID (none is all music). */
export const chartsSearch = z.object({
  genre: z.string().optional().catch(undefined),
});

/** /charts/$chart?genre=…&chartId=… : one chart in full. */
export const chartSearch = z.object({
  genre: z.string().optional().catch(undefined),
  chartId: z.string().optional().catch(undefined),
});
