import type { OnAir } from "@bindings/services";
import { StationUnavailable } from "@bindings/services/models";
import { t } from "@lingui/core/macro";

/** Why Sona can't play a station (Station.unavailable), in words; "" when it can. */
export function stationUnavailable(reason: StationUnavailable): string {
  switch (reason) {
    case StationUnavailable.StationNoWidevine:
      return t`This stream isn't offered with Widevine, the only protection Sona can play.`;
    case StationUnavailable.$zero:
      return "";
  }
}

/** The song a broadcaster has on air, as one line: "Artist – Title". */
export function onAirLine(on: Pick<OnAir, "artist" | "title">): string {
  return on.artist ? `${on.artist} – ${on.title}` : on.title;
}
