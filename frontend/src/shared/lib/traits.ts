import type { Traits } from "@bindings/services";

/** Apple's audio labels, most specific first. Labels only: Sona streams AAC. */
export function traitLabels(t: Traits | undefined): string[] {
  if (!t) return [];
  const out: string[] = [];
  if (t.atmos) out.push("Dolby Atmos");
  else if (t.spatial) out.push("Spatial Audio");
  if (t.hiResLossless) out.push("Hi-Res Lossless");
  else if (t.lossless) out.push("Lossless");
  if (t.appleDigitalMaster) out.push("Apple Digital Master");
  return out;
}
