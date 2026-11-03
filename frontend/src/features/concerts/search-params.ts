import { z } from "zod";

import { whenValues } from "./dates";

/**
 * /concerts?when=…&from=…&to=…&genre=…&yours=1: the date filter (a preset,
 * or "dates" with `from` and `to` as "2026-11-14"), an Apple genre ID, and
 * only the user's own artists. Anything unreadable is dropped.
 */
export const concertsSearch = z.object({
  when: z.enum(whenValues).optional().catch(undefined),
  from: z
    .string()
    .regex(/^\d{4}-\d{2}-\d{2}$/)
    .optional()
    .catch(undefined),
  to: z
    .string()
    .regex(/^\d{4}-\d{2}-\d{2}$/)
    .optional()
    .catch(undefined),
  genre: z.string().regex(/^\d+$/).optional().catch(undefined),
  yours: z.literal(true).optional().catch(undefined),
});
export type ConcertsSearch = z.infer<typeof concertsSearch>;
