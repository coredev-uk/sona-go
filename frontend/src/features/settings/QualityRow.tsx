import { Trans, useLingui } from "@lingui/react/macro";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";

import { useSettings, useUpdateSettings } from "./queries";
import { SELECT_WIDTH, SettingRow } from "./SettingRow";

/** Settings → Playback: AAC 256 or Data saver. */
export function QualityRow() {
  const { t } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  const s = settings.data;
  if (!s) return null;
  return (
    <SettingRow
      id="quality"
      title={t`Audio quality`}
      description={t`AAC at 256 kbps, or 64 kbps with Data saver for slow or metered connections. Applies from the next song. Lossless and Dolby Atmos albums play as AAC too.`}
    >
      <Select
        value={String(s.audioQuality)}
        onValueChange={(v) => update.mutate({ ...s, audioQuality: Number(v) })}
      >
        <SelectTrigger id="quality" aria-describedby="quality-desc" className={SELECT_WIDTH}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="256">
            <Trans>High · 256 kbps</Trans>
          </SelectItem>
          <SelectItem value="64">
            <Trans>Data saver · 64 kbps</Trans>
          </SelectItem>
        </SelectContent>
      </Select>
    </SettingRow>
  );
}
