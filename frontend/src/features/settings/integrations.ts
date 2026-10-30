import { IntegrationsService } from "@bindings/services";
import { useQuery } from "@tanstack/react-query";

/** The integrations' live state; Go's integrations:status replaces it. */
export const integrationsStatusKey = ["integrations", "status"] as const;

export function useIntegrationsStatus() {
  return useQuery({
    queryKey: integrationsStatusKey,
    queryFn: () => IntegrationsService.Status(),
  });
}
