import { Trans, useLingui } from "@lingui/react/macro";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";

import { useSettings, useUpdateSettings } from "./queries";
import { SELECT_WIDTH, SettingRow } from "./SettingRow";

/** Settings → Playback: what happens when playing something would replace a queue you built. */
export function QueueRows() {
  const { t } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  const s = settings.data;
  if (!s) return null;
  return (
    <SettingRow
      id="replace-queue"
      title={t`Replacing your queue`}
      description={t`When you play an album, playlist or station while more than three songs you added are waiting in the queue.`}
    >
      <Select
        value={s.queue.replaceConfirm}
        onValueChange={(v) => update.mutate({ ...s, queue: { ...s.queue, replaceConfirm: v } })}
      >
        <SelectTrigger
          id="replace-queue"
          aria-describedby="replace-queue-desc"
          className={SELECT_WIDTH}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="undo">
            <Trans>Replace, offer Undo</Trans>
          </SelectItem>
          <SelectItem value="confirm">
            <Trans>Ask first</Trans>
          </SelectItem>
        </SelectContent>
      </Select>
    </SettingRow>
  );
}
