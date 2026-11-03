import { useLingui } from "@lingui/react/macro";
import { Switch } from "@/shared/ui/switch";

import { useAppearance, type Theme } from "./appearance";
import { useSettings, useUpdateSettings } from "./queries";
import { MotionArtworkRows } from "./MotionArtworkRows";
import { SettingRow, SettingsPanel } from "./SettingRow";

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

/**
 * The theme as three segments, all in view at once (radio buttons: ←/→
 * move and choose, one Tab stop), instead of a menu that hides two of
 * three choices.
 */
function ThemeChoice({
  value,
  disabled,
  onChange,
}: {
  value: Theme;
  disabled: boolean;
  onChange: (theme: Theme) => void;
}) {
  const { t } = useLingui();
  const choices: { value: Theme; label: string }[] = [
    { value: "system", label: t`System` },
    { value: "light", label: t`Light` },
    { value: "dark", label: t`Dark` },
  ];
  return (
    <div
      id="theme"
      role="radiogroup"
      aria-labelledby="theme-label"
      aria-describedby="theme-desc"
      className="flex h-8 rounded-md border border-control-line p-0.5"
    >
      {choices.map((c) => (
        <label
          key={c.value}
          className="relative flex rounded-[4px] has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-1 has-[:focus-visible]:outline-[var(--ring)] has-[:focus-visible]:outline-solid"
        >
          <input
            type="radio"
            name="theme"
            value={c.value}
            checked={value === c.value}
            disabled={disabled}
            onChange={() => onChange(c.value)}
            className="peer sr-only"
          />
          <span className="flex min-w-16 cursor-default items-center justify-center rounded-[4px] px-3 text-sub text-ink-muted transition-colors duration-100 peer-checked:bg-active peer-checked:font-medium peer-checked:text-ink peer-disabled:opacity-50 hover:text-ink">
            {c.label}
          </span>
        </label>
      ))}
    </div>
  );
}

export function AppearanceSection() {
  const { t } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  const theme = useAppearance((s) => s.theme);
  return (
    <SettingsPanel>
      <SettingRow
        id="theme"
        title={t`Theme`}
        description={t`System follows your desktop's light or dark preference.`}
      >
        <ThemeChoice
          value={theme}
          disabled={!settings.data}
          onChange={(next) => {
            if (settings.data) update.mutate({ ...settings.data, theme: next });
          }}
        />
      </SettingRow>
      <ReduceEffectsRow />
      <MotionArtworkRows />
    </SettingsPanel>
  );
}
