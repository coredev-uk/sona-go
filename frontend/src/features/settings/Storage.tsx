import { plural } from "@lingui/core/macro";
import { Plural, Trans, useLingui } from "@lingui/react/macro";
import { StorageService } from "@bindings/services";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { callFailure } from "@/shared/lib/errors";
import { formatBytes, relativeTime } from "@/shared/lib/format";
import { keys } from "@/shared/lib/keys";
import { Button } from "@/shared/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";

import { useSettings, useUpdateSettings } from "./queries";
import { SettingRow, SettingsPanel } from "./SettingRow";
import { SettingsFileRows } from "./SettingsFileRows";

/** Artwork cache limits offered, in MiB. */
const artChoices = [256, 512, 1024, 2048, 4096];

/** Sizes of the artwork cache, the catalogue cache and the database; their limit, clearing and compacting. */
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
  if (!s || !u) return <SettingsPanel className="h-80" />;
  const limit = s.storage.artCacheMiB;
  const choices = artChoices.includes(limit)
    ? artChoices
    : [...artChoices, limit].sort((a, b) => a - b);
  const artworkUsed = formatBytes(u.artworkBytes);
  const artworkLimit = mib(limit);
  const catalogEntries = u.catalogEntries;
  const catalogSize = formatBytes(u.catalogBytes);
  const lastMaintenance = u.lastMaintenance ? relativeTime(u.lastMaintenance) : "";
  const databaseSize = formatBytes(u.databaseBytes);
  const databaseFree = formatBytes(u.databaseFreeBytes);

  return (
    <SettingsPanel>
      <SettingRow
        id="art-cache"
        title={t`Artwork cache`}
        description={
          <>
            <Trans>
              Covers and backdrops kept on this computer so pages open without downloading them
              again. Above the limit, the artwork shown least recently is removed first.
            </Trans>{" "}
            <span className="text-ink num">
              <Trans>
                {artworkUsed} of {artworkLimit} used.
              </Trans>
            </span>
          </>
        }
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
          <SelectTrigger id="art-cache" aria-describedby="art-cache-desc" className="h-8 w-44">
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
        description={
          <>
            <Trans>
              Apple Music pages kept so they open at once and while offline; expired pages are
              removed every few hours.
            </Trans>{" "}
            <span className="text-ink num">
              <Plural
                value={catalogEntries}
                one={`# page, ${catalogSize}.`}
                other={`# pages, ${catalogSize}.`}
              />
            </span>{" "}
            <Trans>Clearing it and the artwork cache keeps your library and settings.</Trans>
          </>
        }
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
          <>
            {lastMaintenance ? (
              <Trans>
                Your synced library, history and the catalogue cache. Sona optimises it when it is
                idle and nothing is playing (last {lastMaintenance}).
              </Trans>
            ) : (
              <Trans>
                Your synced library, history and the catalogue cache. Sona optimises it when it is
                idle and nothing is playing.
              </Trans>
            )}{" "}
            <span className="text-ink num">
              {u.databaseFreeBytes > 0 ? (
                <Trans>
                  {databaseSize}, of which {databaseFree} is free space.
                </Trans>
              ) : (
                <Trans>{databaseSize}.</Trans>
              )}
            </span>
          </>
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
  );
}
