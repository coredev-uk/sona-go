import { Trans, useLingui } from "@lingui/react/macro";

import { MotionArtworkRows, ReduceEffectsRow, SettingsGroup } from "@/features/settings";

import { StepHeader } from "./parts";
import { ThemePicker } from "./ThemePicker";

/** Appearance: the theme as pictures, then reduce effects and motion artwork as in Settings. */
export function AppearanceStep() {
  const { t } = useLingui();
  return (
    <>
      <StepHeader title={t`Appearance`}>
        <Trans>How Sona looks. Each choice applies at once.</Trans>
      </StepHeader>
      <div className="flex flex-col gap-8">
        <ThemePicker />
        <SettingsGroup id="setup-effects" title={t`Effects`}>
          <ReduceEffectsRow />
          <MotionArtworkRows />
        </SettingsGroup>
      </div>
    </>
  );
}
