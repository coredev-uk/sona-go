import { createContext } from "react";

/**
 * The level of a settings group's heading: 3 under Settings' section
 * heading (h2), 2 where the rows sit right under a page's h1 (the
 * first-run setup).
 */
export const GroupHeadingLevel = createContext<2 | 3>(3);
