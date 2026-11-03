import { toast } from "sonner";

/**
 * Says why a command that doesn't apply right now did nothing ("Not
 * available while listening to a live station"): politely, in the
 * notifications region, one message at a time however fast the key repeats.
 */
export function announceUnavailable(reason: string) {
  toast(reason, { id: "command-unavailable" });
}
