import { AudioService } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";

import { useCapabilities } from "@/shared/lib/capabilities";
import { Button } from "@/shared/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Switch } from "@/shared/ui/switch";

import { useSettings, useUpdateSettings } from "./queries";
import { SettingRow } from "./SettingRow";

/** The value standing for "follow the system default" (Radix Select needs a non-empty value). */
const SYSTEM = "system-default";

function OutputRow() {
  const { t } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  // PipeWire sinks come and go (headsets, docks): re-list when the window
  // regains focus and while the menu is open.
  const devices = useQuery({
    queryKey: ["audio", "outputs"],
    queryFn: () => AudioService.OutputDevices(),
    refetchOnWindowFocus: "always",
  });
  const s = settings.data;
  if (!s) return null;
  const list = devices.data ?? [];
  const chosen = s.audio.outputDevice;
  const fallback = list.find((d) => d.default);
  const missing = chosen !== "" && !list.some((d) => d.name === chosen);
  const fallbackName = fallback?.description ?? "";
  return (
    <SettingRow
      id="output"
      title={t`Output`}
      description={
        missing
          ? t`The output Sona was set to isn't connected; it plays to the system default until it's back.`
          : t`Where Sona plays. Switching moves the music that's playing without a pause.`
      }
    >
      <Select
        value={chosen || SYSTEM}
        onOpenChange={(open) => open && void devices.refetch()}
        onValueChange={(v) =>
          update.mutate({ ...s, audio: { ...s.audio, outputDevice: v === SYSTEM ? "" : v } })
        }
      >
        <SelectTrigger id="output" aria-describedby="output-desc" className="h-8 w-60">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={SYSTEM}>
            {fallback ? (
              <Trans>System default · {fallbackName}</Trans>
            ) : (
              <Trans>System default</Trans>
            )}
          </SelectItem>
          {list.map((d) => (
            <SelectItem key={d.name} value={d.name}>
              {d.description}
            </SelectItem>
          ))}
          {missing && (
            <SelectItem value={chosen} disabled>
              <Trans>{chosen} · not connected</Trans>
            </SelectItem>
          )}
        </SelectContent>
      </Select>
    </SettingRow>
  );
}

/**
 * Settings → Playback: the output device, Sound Check and the way to the
 * equaliser, each only where the platform's playback supports it.
 */
export function AudioRows() {
  const { t } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  const playback = useCapabilities()?.playback;
  const s = settings.data;
  if (!s || !playback) return null;
  const eq = s.audio.equalizer;
  return (
    <>
      {playback.outputDevice && <OutputRow />}
      {playback.soundCheck && (
        <SettingRow
          id="sound-check"
          title={t`Sound Check`}
          description={t`Plays every song at about the same loudness, using the loudness Apple measured for it. Uploaded songs use their own Sound Check tag, if iTunes or Music wrote one.`}
        >
          <Switch
            id="sound-check"
            aria-describedby="sound-check-desc"
            checked={s.audio.soundCheck}
            onCheckedChange={(soundCheck) =>
              update.mutate({ ...s, audio: { ...s.audio, soundCheck } })
            }
          />
        </SettingRow>
      )}
      {playback.equalizer && (
        <SettingRow
          id="equalizer"
          title={t`Equaliser`}
          description={
            eq.enabled
              ? t`Ten bands, presets and a preamp, heard as you move them. On now.`
              : t`Ten bands, presets and a preamp, heard as you move them. Off now.`
          }
        >
          <Button
            id="equalizer"
            aria-describedby="equalizer-desc"
            variant="outline"
            size="sm"
            asChild
          >
            <Link to="/equalizer">
              <Trans>Open equaliser</Trans>
            </Link>
          </Button>
        </SettingRow>
      )}
    </>
  );
}
