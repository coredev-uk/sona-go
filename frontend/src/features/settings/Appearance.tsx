import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Switch } from "@/shared/ui/switch";

import { useAppearance, type Theme } from "./appearance";
import { useSettings, useUpdateSettings } from "./queries";
import { SettingRow, SettingsPanel } from "./SettingRow";

export function AppearanceSection() {
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
        title="Theme"
        description="System follows your desktop's light or dark preference."
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
            <SelectItem value="system">System</SelectItem>
            <SelectItem value="light">Light</SelectItem>
            <SelectItem value="dark">Dark</SelectItem>
          </SelectContent>
        </Select>
      </SettingRow>
      <SettingRow
        id="reduce-effects"
        title="Reduce effects"
        description="Solid surfaces instead of artwork colour washes, and no animation. Smoother on software rendering."
      >
        <Switch
          id="reduce-effects"
          aria-describedby="reduce-effects-desc"
          checked={reduceEffects}
          disabled={!settings.data}
          onCheckedChange={(on) => save({ reduceEffects: on })}
        />
      </SettingRow>
    </SettingsPanel>
  );
}
