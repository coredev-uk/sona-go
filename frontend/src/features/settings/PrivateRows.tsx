import { useLingui } from "@lingui/react/macro";

import { isForced, setPrivate, usePrivate } from "@/features/private";
import { Switch } from "@/shared/ui/switch";

import { SettingRow } from "./SettingRow";

/**
 * Settings → Playback: private listening, the same switch as the player
 * bar's. The description says where a state the user did not choose comes
 * from: a development build (switching lasts this session; the saved
 * choice is untouched) or SONA_PRIVATE=1 (cannot be switched off).
 */
export function PrivateRows() {
  const { t } = useLingui();
  const st = usePrivate((s) => s.state);
  const forced = isForced(st);
  const description = forced
    ? t`On because Sona was started with SONA_PRIVATE=1. Start Sona without it to turn private listening off.`
    : st?.source === "development"
      ? st.saved
        ? t`This development build of Sona starts with private listening on. Switching it here lasts until Sona quits; your saved choice (on) is unchanged.`
        : t`This development build of Sona starts with private listening on. Switching it here lasts until Sona quits; your saved choice (off) is unchanged.`
      : t`Plays stay out of your Apple Music listening history and recommendations, and aren't scrobbled to Last.fm or ListenBrainz, shown on Discord or added to History. The same as the button in the player bar.`;
  return (
    <SettingRow
      id="private-listening"
      title={t({
        message: "Private listening",
        comment:
          "Setting name. No Apple Music web string; Apple's apps offer the inverse, 'Use Listening History'.",
      })}
      description={description}
    >
      <Switch
        id="private-listening"
        aria-describedby="private-listening-desc"
        checked={st?.on ?? false}
        disabled={!st || forced}
        onCheckedChange={(on) => void setPrivate(on)}
      />
    </SettingRow>
  );
}
