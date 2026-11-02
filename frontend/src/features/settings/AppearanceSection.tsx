import { Trans, useLingui } from "@lingui/react/macro";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Switch } from "@/shared/ui/switch";

import { useAppearance, type Theme } from "./appearance";
import { useSettings, useUpdateSettings } from "./queries";
import { MotionArtworkRows } from "./MotionArtworkRows";
import { SELECT_WIDTH, SettingRow, SettingsPanel } from "./SettingRow";

/** Reduce effects (Settings → Appearance). */
export function ReduceEffectsRow() {
  const { t } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  const reduceEffects = useAppearance((s) => s.reduceEffects);
  return (
    <SettingRow
      id="reduce-effects"
      title={t`Reduce effects`}
      description={t`Solid surfaces instead of artwork colour washes, and no animation. Smoother on software rendering.`}
    >
      <Switch
        id="reduce-effects"
        aria-describedby="reduce-effects-desc"
        checked={reduceEffects}
        disabled={!settings.data}
        onCheckedChange={(on) => {
          if (settings.data) update.mutate({ ...settings.data, reduceEffects: on });
        }}
      />
    </SettingRow>
  );
}

export function AppearanceSection() {
  const { t } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  const theme = useAppearance((s) => s.theme);
  const save = (next: Theme) => {
    if (settings.data) update.mutate({ ...settings.data, theme: next });
  };
  return (
    <SettingsPanel>
      <SettingRow
        id="theme"
        title={t`Theme`}
        description={t`System follows your desktop's light or dark preference.`}
      >
        <Select value={theme} onValueChange={(v) => save(v as Theme)} disabled={!settings.data}>
          <SelectTrigger id="theme" aria-describedby="theme-desc" className={SELECT_WIDTH}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="system">
              <Trans>System</Trans>
            </SelectItem>
            <SelectItem value="light">
              <Trans>Light</Trans>
            </SelectItem>
            <SelectItem value="dark">
              <Trans>Dark</Trans>
            </SelectItem>
          </SelectContent>
        </Select>
      </SettingRow>
      <ReduceEffectsRow />
      <MotionArtworkRows />
    </SettingsPanel>
  );
}
