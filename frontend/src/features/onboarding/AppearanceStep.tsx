import { useLingui } from "@lingui/react/macro";

import { useSettings, useUpdateSettings } from "@/features/settings";
import { useCapabilities } from "@/shared/lib/capabilities";

import { SwitchList, SwitchRow } from "./parts";
import { ThemePicker } from "./ThemePicker";

/**
 * Appearance: the theme as pictures, then reduce effects and motion
 * artwork. Each applies at once, to the app behind the setup too.
 */
export function AppearanceStep() {
  const { t } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  // Without the power state, motion artwork can't pause on battery.
  const followsBattery = useCapabilities()?.powerState !== false;
  const s = settings.data;
  const reduce = s?.reduceEffects ?? false;
  return (
    <div className="flex flex-col gap-5">
      <ThemePicker />
      <SwitchList>
        <SwitchRow
          id="setup-reduce-effects"
          label={t`Reduce effects`}
          hint={t`Solid surfaces and no animation. Smoother on software rendering.`}
          checked={reduce}
          disabled={!s}
          onChange={(reduceEffects) => {
            if (s) update.mutate({ ...s, reduceEffects });
          }}
        />
        <SwitchRow
          id="setup-motion-artwork"
          label={t`Motion artwork`}
          hint={
            reduce
              ? t`Off while Reduce effects is on.`
              : followsBattery
                ? t`Plays the looping covers some albums have. Paused on battery power.`
                : t`Plays the looping covers some albums have.`
          }
          checked={(s?.motionArtwork.enabled ?? false) && !reduce}
          disabled={!s || reduce}
          onChange={(enabled) => {
            if (s) update.mutate({ ...s, motionArtwork: { ...s.motionArtwork, enabled } });
          }}
        />
      </SwitchList>
    </div>
  );
}
