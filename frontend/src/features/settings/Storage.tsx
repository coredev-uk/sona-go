import { plural } from "@lingui/core/macro";
import { Plural, Trans, useLingui } from "@lingui/react/macro";
import { StorageService } from "@bindings/services";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { toast } from "sonner";

import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { callFailure } from "@/shared/lib/errors";
import { formatBytes, relativeTime } from "@/shared/lib/format";
import { keys } from "@/shared/lib/keys";
import { Button } from "@/shared/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";

import { useSettings, useUpdateSettings } from "./queries";
import {
  SELECT_WIDTH,
  SettingRow,
  SettingsLoading,
  SettingsPanel,
  SettingsSections,
} from "./SettingRow";
import { SettingsFileRows } from "./SettingsFileRows";

/** Artwork cache limits offered, in MiB. */
const artChoices = [256, 512, 1024, 2048, 4096];

/** One figure of the usage line: what it is, its size (and of what), and a line under it. */
function Figure({
  label,
  value,
  of,
  children,
}: {
  label: string;
  value: string;
  /** The limit the size counts against ("of 512 MB"). */
  of?: string;
  children?: ReactNode;
}) {
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <dt className="text-meta text-ink-muted">{label}</dt>
      <dd className="text-heading text-ink num">
        {value}
        {of && <span className="text-sub font-normal text-ink-muted"> {of}</span>}
      </dd>
      {children && <dd className="text-meta text-ink-muted num">{children}</dd>}
    </div>
  );
}

/**
 * Settings → Storage: what Sona keeps on this computer first, as figures
 * (the artwork cache against its limit, the catalogue cache, the
 * database), then the limit, clearing, compacting and the settings file.
 */
export function StorageSection() {
  const { t } = useLingui();
  const mib = (n: number) => {
    const gb = n / 1024;
    return n >= 1024 ? t`${gb} GB` : t`${n} MB`;
  };
  const qc = useQueryClient();
  const settings = useSettings();
  const update = useUpdateSettings();
  const usage = useQuery({ queryKey: keys.storage, queryFn: () => StorageService.Usage() });
  const refresh = () => void qc.invalidateQueries({ queryKey: keys.storage });
  const clear = useMutation({
    mutationFn: () => StorageService.Clear(),
    onSuccess: (r) => {
      const artwork = formatBytes(r.artworkBytes);
      const pages = r.catalogEntries;
      toast(
        t`Cleared ${artwork} of artwork and ${plural(pages, { one: "# cached page", other: "# cached pages" })}`,
      );
    },
    onError: (err) =>
      toast.error(t`Couldn't clear the cache`, { description: callFailure(err).message }),
    onSettled: refresh,
  });
  const compact = useMutation({
    mutationFn: () => StorageService.Compact(),
    onSuccess: (freed) => {
      const size = formatBytes(freed);
      toast(freed > 0 ? t`Gave back ${size} to the disk` : t`The database was already compact`);
    },
    onError: (err) =>
      toast.error(t`Couldn't compact the database`, { description: callFailure(err).message }),
    onSettled: refresh,
  });

  const error = settings.error ?? usage.error;
  if (error) {
    return (
      <ErrorBlock
        title={t`Couldn't read Sona's storage`}
        error={error}
        onRetry={() => {
          void settings.refetch();
          void usage.refetch();
        }}
      />
    );
  }
  const s = settings.data;
  const u = usage.data;
  if (!s || !u) return <SettingsLoading rows={4} />;
  const limit = s.storage.artCacheMiB;
  const choices = artChoices.includes(limit)
    ? artChoices
    : [...artChoices, limit].sort((a, b) => a - b);
  const artworkUsed = formatBytes(u.artworkBytes);
  const artworkLimit = mib(limit);
  const share = Math.min(1, u.artworkBytes / (limit * 1024 * 1024));
  const percent = Math.round(share * 100);
  const catalogEntries = u.catalogEntries;
  const catalogSize = formatBytes(u.catalogBytes);
  const lastMaintenance = u.lastMaintenance ? relativeTime(u.lastMaintenance) : "";
  const databaseSize = formatBytes(u.databaseBytes);
  const databaseFree = formatBytes(u.databaseFreeBytes);

  return (
    <SettingsSections>
      <dl className="grid grid-cols-3 gap-6 border-t border-line pt-4">
        <Figure label={t`Artwork cache`} value={artworkUsed} of={t`of ${artworkLimit}`}>
          <span
            role="meter"
            aria-label={t`Artwork cache`}
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={percent}
            aria-valuetext={t`${artworkUsed} of ${artworkLimit}`}
            className="mt-1.5 block h-1 w-full max-w-48 overflow-hidden rounded-full bg-hover"
          >
            <span className="block h-full bg-ink-muted" style={{ width: `${share * 100}%` }} />
          </span>
        </Figure>
        <Figure label={t`Catalogue cache`} value={catalogSize}>
          <Plural value={catalogEntries} one="# page" other="# pages" />
        </Figure>
        <Figure label={t`Library database`} value={databaseSize}>
          {u.databaseFreeBytes > 0 && <Trans>{databaseFree} of it free space</Trans>}
        </Figure>
      </dl>
      <SettingsPanel>
        <SettingRow
          id="art-cache"
          title={t`Artwork cache`}
          description={t`Covers and backdrops kept on this computer so pages open without downloading them again. Above the limit, the artwork shown least recently is removed first.`}
        >
          <Select
            value={String(limit)}
            onValueChange={(v) =>
              // Go evicts down to a lower limit before the save returns.
              update.mutate(
                { ...s, storage: { ...s.storage, artCacheMiB: Number(v) } },
                { onSuccess: refresh },
              )
            }
          >
            <SelectTrigger
              id="art-cache"
              aria-describedby="art-cache-desc"
              className={SELECT_WIDTH}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {choices.map((n) => {
                const size = mib(n);
                return (
                  <SelectItem key={n} value={String(n)}>
                    {t`Up to ${size}`}
                  </SelectItem>
                );
              })}
            </SelectContent>
          </Select>
        </SettingRow>
        <SettingRow
          id="clear-cache"
          title={t`Catalogue cache`}
          description={t`Apple Music pages kept so they open at once and while offline; expired pages are removed every few hours. Clearing it and the artwork cache keeps your library and settings.`}
        >
          <Button
            id="clear-cache"
            aria-describedby="clear-cache-desc"
            variant="outline"
            size="sm"
            disabled={clear.isPending}
            onClick={() => clear.mutate()}
          >
            {clear.isPending ? <Trans>Clearing…</Trans> : <Trans>Clear caches</Trans>}
          </Button>
        </SettingRow>
        <SettingRow
          id="compact"
          title={t`Library database`}
          description={
            lastMaintenance ? (
              <Trans>
                Your synced library, history and the catalogue cache. Sona optimises it when it is
                idle and nothing is playing (last {lastMaintenance}).
              </Trans>
            ) : (
              <Trans>
                Your synced library, history and the catalogue cache. Sona optimises it when it is
                idle and nothing is playing.
              </Trans>
            )
          }
        >
          <Button
            id="compact"
            aria-describedby="compact-desc"
            variant="outline"
            size="sm"
            disabled={compact.isPending}
            onClick={() => compact.mutate()}
          >
            {compact.isPending ? <Trans>Compacting…</Trans> : <Trans>Compact now</Trans>}
          </Button>
        </SettingRow>
        <SettingsFileRows />
      </SettingsPanel>
    </SettingsSections>
  );
}
