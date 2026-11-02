import { plural, t } from "@lingui/core/macro";
import type { MotionReport, MotionStatus, PowerState } from "@bindings/services";

import { codeMessage } from "@/shared/lib/errors";
import { formatBytes, relativeTime } from "@/shared/lib/format";
import type { MotionAttempt, MotionState } from "@/shared/lib/motion";

import type { Group, Row } from "./Diagnostics";

/** Why motion is on or off, from the first input that stops it. */
function verdict(s: MotionState): Row {
  const label = t`Motion artwork`;
  if (s.allowed) return { label, value: t`Plays where Apple has it`, status: "ok" };
  let why: string;
  if (!s.setting) why = t`Off in Settings → Appearance`;
  else if (s.reduceEffects) why = t`Off while Reduce effects is on`;
  else if (s.reducedMotion) why = t`Off: your system asks apps to reduce motion`;
  else if (s.power?.onBattery) why = t`Paused on battery`;
  else why = t`Off: WebKit can't decode Apple's videos`;
  return { label, value: why, status: s.codec === "" ? "error" : undefined };
}

/** The power source Go decided, and why. */
function powerRow(p: PowerState | null): Row {
  const label = t`Power`;
  if (!p) return { label, value: t`Checking…` };
  if (!p.followed) {
    return p.unreadable
      ? {
          label,
          value: t`Unknown: counted as mains power`,
          status: "warning",
          detail: t`UPower didn't answer. Sona's log says why.`,
        }
      : { label, value: t`Not followed on this platform: counted as mains power` };
  }
  const notes: string[] = [];
  if (p.ignored > 0)
    notes.push(
      plural(p.ignored, {
        one: "# peripheral battery (a mouse, keyboard or headset) is ignored.",
        other: "# peripheral batteries (mice, keyboards, headsets) are ignored.",
      }),
    );
  if (p.upower !== p.onBattery)
    notes.push(
      p.upower
        ? t`UPower itself reports this computer on battery; Sona decides from the power devices instead.`
        : t`UPower itself reports mains power; Sona decides from the power devices instead.`,
    );
  const detail = notes.join(" ") || undefined;
  switch (p.source) {
    case "ac":
      return { label, value: t`Mains power (AC adapter connected)`, status: "ok", detail };
    case "battery":
      return { label, value: t`On battery: motion artwork pauses`, status: "warning", detail };
    case "ups":
      return { label, value: t`On UPS battery: motion artwork pauses`, status: "warning", detail };
    default:
      return {
        label,
        value: t`Mains power (no battery powers this computer)`,
        status: "ok",
        detail,
      };
  }
}

/** The newest motion video in this window. */
function lastVideo(a: MotionAttempt | null): Row {
  const label = t`Last video`;
  if (!a) return { label, value: t`None since Sona started` };
  const when = relativeTime(a.at);
  switch (a.outcome) {
    case "loading":
      return { label, value: t`Loading · ${when}` };
    case "playing":
      return { label, value: t`Played · ${when}`, status: "ok" };
    case "download-failed":
      return { label, value: t`Couldn't download · ${when}`, status: "warning", detail: a.detail };
    case "media-error":
      return {
        label,
        value: t`WebKit couldn't play it · ${when}`,
        status: "warning",
        detail: a.detail,
      };
    case "stalled":
      return { label, value: t`Stalled · ${when}`, status: "warning", detail: a.detail };
  }
}

/** The newest rendition Go downloaded from Apple. */
function lastDownload(s: MotionStatus | undefined): Row {
  const label = t`Last download`;
  if (!s) return { label, value: t`Checking…` };
  const l = s.last;
  if (!l.at) return { label, value: t`None since Sona started` };
  const when = relativeTime(l.at);
  if (l.error)
    return {
      label,
      value: t`Failed · ${when}`,
      status: "warning",
      detail: l.rendition ? `${l.rendition}: ${codeMessage(l.error)}` : codeMessage(l.error),
    };
  return { label, value: `${l.rendition} · ${when}`, status: "ok" };
}

/**
 * Settings → Diagnostics → Motion artwork: every input that decides whether
 * motion plays, what WebKit can decode, and how the newest video and
 * download went. Null without the motion module (nothing probed).
 */
export function motionGroup(
  s: MotionState,
  last: MotionAttempt | null,
  status: MotionStatus | undefined,
  visible: boolean,
): Group | null {
  if (!s.probed) return null;
  const decoderHelp = t`Install gst-libav (avdec_h264) and gst-plugins-bad (h264parse) for WebKit's GStreamer, then restart Sona.`;
  const rows: Row[] = [
    verdict(s),
    { label: t`Setting`, value: s.setting ? t`On` : t`Off` },
    { label: t`Reduce effects`, value: s.reduceEffects ? t`On` : t`Off` },
    {
      label: t`System reduced motion`,
      value: s.reducedMotion ? t`Requested` : t`Not requested`,
      detail: s.reducedMotion
        ? t`Your desktop asks apps to reduce motion (GTK's gtk-enable-animations is off).`
        : undefined,
    },
    powerRow(s.power),
    s.h264
      ? { label: t`H.264 decoding`, value: t`Supported`, status: "ok" }
      : {
          label: t`H.264 decoding`,
          value: t`Not supported`,
          status: s.hevc ? "warning" : "error",
          detail: s.hevc ? t`HEVC is used instead. ${decoderHelp}` : decoderHelp,
        },
    { label: t`HEVC decoding`, value: s.hevc ? t`Supported` : t`Not supported` },
    { label: t`Window`, value: visible ? t`Visible` : t`Hidden: videos pause` },
    lastVideo(last),
    lastDownload(status),
  ];
  if (status)
    rows.push({
      label: t`Motion cache`,
      value: `${plural(status.cacheFiles, { one: "# video", other: "# videos" })} · ${formatBytes(status.cacheBytes)}`,
    });
  return { title: t`Motion artwork`, rows };
}

/** The web view's side for the bug-report bundle (MotionService.Report). */
export function motionReport(
  s: MotionState,
  last: MotionAttempt | null,
  playing: number,
  visible: boolean,
): MotionReport {
  return {
    allowed: s.allowed,
    setting: s.setting,
    reduceEffects: s.reduceEffects,
    reducedMotion: s.reducedMotion,
    onBattery: s.power?.onBattery ?? false,
    h264: s.h264,
    hevc: s.hevc,
    visible,
    playing,
    last: last
      ? `${last.outcome} ${new Date(last.at).toISOString()} ${last.src} ${last.detail}`.trim()
      : "",
  };
}
