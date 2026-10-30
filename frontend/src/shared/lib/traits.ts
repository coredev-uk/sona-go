import type { Traits } from "@bindings/services";
import { t } from "@lingui/core/macro";

/** Apple's audio labels, most specific first. Labels only: Sona streams AAC. */
export function traitLabels(traits: Traits | undefined): string[] {
  if (!traits) return [];
  const out: string[] = [];
  if (traits.atmos) out.push(t`Dolby Atmos`);
  else if (traits.spatial) out.push(t`Spatial Audio`);
  if (traits.hiResLossless) out.push(t`Hi-Res Lossless`);
  else if (traits.lossless) out.push(t`Lossless`);
  if (traits.appleDigitalMaster) out.push(t`Apple Digital Master`);
  return out;
}
