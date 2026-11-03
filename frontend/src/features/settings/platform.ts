import { useLingui } from "@lingui/react/macro";

import { useCapabilities } from "@/shared/lib/capabilities";

/** The platform's name as people know it, from `Capabilities.os`. */
export function usePlatformName(): string {
  const { t } = useLingui();
  const os = useCapabilities()?.os ?? "";
  switch (os) {
    case "darwin":
      return t`macOS`;
    case "windows":
      return t`Windows`;
    case "linux":
      return t`Linux`;
    default:
      return os;
  }
}
