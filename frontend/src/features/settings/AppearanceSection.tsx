import { Trans, useLingui } from "@lingui/react/macro";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Switch } from "@/shared/ui/switch";

import { useAppearance, type Theme } from "./appearance";
import { useSettings, useUpdateSettings } from "./queries";
import { MotionArtworkRows } from "./MotionArtworkRows";
import { SettingRow, SettingsPanel } from "./SettingRow";

export function AppearanceSection() {
  const { t } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  const { theme, reduceEffects } = useAppearance();
  const save = (patch: { theme?: Theme; reduceEffects?: boolean }) => {
    if (settings.data) update.mutate({ ...settings.data, ...patch });
  };
  return (
    <SettingsPanel>
      <SettingRow
        id="theme"
        title={t`Theme`}
        description={t`System follows your desktop's light or dark preference.`}
      >
        <Select
          value={theme}
          onValueChange={(v) => save({ theme: v as Theme })}
          disabled={!settings.data}
        >
          <SelectTrigger id="theme" aria-describedby="theme-desc" className="h-8 w-36">
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
          onCheckedChange={(on) => save({ reduceEffects: on })}
        />
      </SettingRow>
      <MotionArtworkRows />
    </SettingsPanel>
  );
}
