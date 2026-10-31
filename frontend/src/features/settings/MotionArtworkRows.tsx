import { useLingui } from "@lingui/react/macro";
import { useCapabilities } from "@/shared/lib/capabilities";
import { Switch } from "@/shared/ui/switch";

import { useAppearance } from "./appearance";
import { useSettings, useUpdateSettings } from "./queries";
import { SettingRow } from "./SettingRow";

/** Settings → Appearance: Apple's motion covers. */
export function MotionArtworkRows() {
  const { t } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  const reduceEffects = useAppearance((s) => s.reduceEffects);
  // Without the power state, motion artwork can't pause on battery.
  const followsBattery = useCapabilities()?.powerState !== false;
  const s = settings.data;
  return (
    <SettingRow
      id="motion-artwork"
      title={t`Motion artwork`}
      description={
        reduceEffects
          ? t`Off while Reduce effects is on.`
          : followsBattery
            ? t`Plays the looping covers some albums and playlists have, in their headers and in Now Playing. Paused on battery power.`
            : t`Plays the looping covers some albums and playlists have, in their headers and in Now Playing.`
      }
    >
      <Switch
        id="motion-artwork"
        aria-describedby="motion-artwork-desc"
        checked={(s?.motionArtwork.enabled ?? false) && !reduceEffects}
        disabled={!s || reduceEffects}
        onCheckedChange={(enabled) => {
          if (s) update.mutate({ ...s, motionArtwork: { ...s.motionArtwork, enabled } });
        }}
      />
    </SettingRow>
  );
}
