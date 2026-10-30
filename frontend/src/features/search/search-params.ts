import { z } from "zod";

export type SearchScope = "catalog" | "library";

const term = z
  .string()
  .trim()
  .transform((q) => q || undefined)
  .optional()
  .catch(undefined);

/** /search?q=…&scope=library — blank queries are dropped; the scope defaults to Apple Music. */
export const searchSearch = z.object({
  q: term,
  scope: z.enum(["catalog", "library"]).optional().catch(undefined),
});

/** /search/$group?q=… — a "see all" page of one result group. */
export const groupSearch = z.object({ q: term });
