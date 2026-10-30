import { Trans, useLingui } from "@lingui/react/macro";
import { UpdatesService, type UpdateStatus } from "@bindings/services";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CircleAlert, CircleCheck, ExternalLink, RotateCw } from "lucide-react";

import { Button } from "@/shared/ui/button";
import { Switch } from "@/shared/ui/switch";
import { callFailure } from "@/shared/lib/errors";
import { relativeTime } from "@/shared/lib/format";
import { keys } from "@/shared/lib/keys";
import { run } from "@/shared/lib/run";

import { useSettings, useUpdateSettings } from "./queries";
import { SettingRow, SettingsPanel } from "./SettingRow";

/** Settings → Diagnostics: the start-up update check and its latest result. */
export function UpdatesGroup() {
  const { t } = useLingui();
  const verdict = (st: UpdateStatus): { text: string; detail?: string; ok?: boolean } => {
    const { current, latest, error } = st;
    if (!st.checkable)
      return {
        text: current
          ? t`Development build (${current}): not checked for updates`
          : t`Development build (no version): not checked for updates`,
      };
    if (st.available)
      return {
        text: t`Sona ${latest} is available`,
        detail: st.appImage
          ? t`Update this AppImage in place with AppImageUpdate, or download the new one from the release page.`
          : t`Download the new package from the release page, or update it with your package manager.`,
      };
    if (error) return { text: t`Couldn't reach GitHub: ${error}`, ok: false };
    if (!st.checked) return { text: t`Sona ${current} · not checked yet` };
    const checked = relativeTime(st.checked);
    return {
      text: latest
        ? t`Sona ${current} is up to date`
        : t`Sona ${current} · no release published yet`,
      detail: t`Checked ${checked}.`,
      ok: true,
    };
  };
  const qc = useQueryClient();
  const settings = useSettings();
  const update = useUpdateSettings();
  const status = useQuery({ queryKey: keys.updates, queryFn: () => UpdatesService.Status() });
  const check = useMutation({
    mutationFn: () => UpdatesService.Check(),
    onSettled: (data, err) => {
      if (data) qc.setQueryData(keys.updates, data);
      else if (err) void qc.invalidateQueries({ queryKey: keys.updates });
    },
  });
  const s = settings.data;
  const st = status.data;
  const v = st && verdict(st);
  const failure = check.error ? callFailure(check.error).message : undefined;

  return (
    <section className="mt-6">
      <h3 className="mb-2 text-ui font-semibold text-ink">
        <Trans>Updates</Trans>
      </h3>
      <SettingsPanel>
        {s && (
          <SettingRow
            id="updates-check"
            title={t`Check for updates when Sona starts`}
            description={t`Asks GitHub for the newest Sona release once the window has opened. Nothing about you, your account or your library is sent.`}
          >
            <Switch
              id="updates-check"
              aria-describedby="updates-check-desc"
              checked={s.updates.check}
              onCheckedChange={(on) =>
                update.mutate({ ...s, updates: { ...s.updates, check: on } })
              }
            />
          </SettingRow>
        )}
        {st && v && (
          <div className="flex items-start justify-between gap-8 py-4" aria-live="polite">
            <div className="min-w-0">
              <p className="flex items-start gap-2 text-sub text-ink">
                {v.ok === true && (
                  <CircleCheck
                    aria-hidden
                    className="mt-px size-4 shrink-0 text-positive"
                    strokeWidth={1.75}
                  />
                )}
                {(v.ok === false || failure) && (
                  <CircleAlert
                    aria-hidden
                    className="mt-px size-4 shrink-0 text-danger"
                    strokeWidth={1.75}
                  />
                )}
                <span className={st.available ? "font-medium" : undefined}>
                  {failure ? t`Couldn't check: ${failure}` : v.text}
                </span>
              </p>
              {v.detail && !failure && (
                <p className="mt-1 max-w-[56ch] text-sub text-ink-muted">{v.detail}</p>
              )}
            </div>
            <div className="flex shrink-0 gap-2">
              {st.available && (
                <Button
                  size="sm"
                  onClick={() =>
                    void run(t`Couldn't open the release`, UpdatesService.OpenRelease())
                  }
                >
                  <ExternalLink strokeWidth={1.75} />
                  <Trans>View release</Trans>
                </Button>
              )}
              {st.checkable && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => check.mutate()}
                  disabled={check.isPending}
                >
                  <RotateCw
                    strokeWidth={1.75}
                    className={check.isPending ? "animate-spin" : undefined}
                  />
                  {check.isPending ? <Trans>Checking…</Trans> : <Trans>Check now</Trans>}
                </Button>
              )}
            </div>
          </div>
        )}
      </SettingsPanel>
    </section>
  );
}
