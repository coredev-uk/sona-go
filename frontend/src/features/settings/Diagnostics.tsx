import { SystemService, type Diagnostics } from "@bindings/services";
import { useQuery } from "@tanstack/react-query";
import { Clipboard } from "@wailsio/runtime";
import { CircleAlert, CircleCheck, Copy, FolderOpen, RotateCw, TriangleAlert } from "lucide-react";
import { toast } from "sonner";

import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { Button } from "@/shared/ui/button";
import { Switch } from "@/shared/ui/switch";
import { run } from "@/shared/lib/run";
import { callFailure } from "@/shared/lib/errors";
import { formatBytes, plural, relativeTime } from "@/shared/lib/format";
import { keys } from "@/shared/lib/keys";
import { cn } from "@/shared/lib/utils";

import { useSettings, useUpdateSettings } from "./queries";

type Status = "ok" | "warning" | "error" | "info";

interface Row {
  label: string;
  value: string;
  status?: Status;
  /** What to do about a warning or error. */
  detail?: string;
}

interface Group {
  title: string;
  rows: Row[];
}

const dash = "—";

/** The Diagnostics struct as labelled rows with a verdict where one applies. */
function diagnosticsGroups(d: Diagnostics): Group[] {
  const p = d.playback;
  const env = (name: string) => d.env.find((e) => e.name === name);
  return [
    {
      title: "Playback",
      rows: [
        p.cdmOk
          ? {
              label: "Widevine CDM",
              value: `Working · ${p.cdmVersion || "version unknown"}`,
              status: "ok",
            }
          : {
              label: "Widevine CDM",
              value: p.cdmPath ? "Doesn't start" : "Not found",
              status: "error",
              detail: p.cdmError
                ? `${p.cdmError}. Install Google Chrome, or set the CDM location under Playback.`
                : "Install Google Chrome, or set the CDM location under Playback. Without it only 30-second previews play.",
            },
        { label: "CDM location", value: p.cdmPath || dash },
        {
          label: "CDM interface",
          value: p.cdmInterface ? `${p.cdmInterface} (host ${p.hostInterface})` : dash,
        },
        {
          label: "GStreamer",
          value: p.gstreamer || "Not found",
          status: p.gstreamer ? "ok" : "error",
          detail: p.gstreamer ? undefined : "Install GStreamer 1.x with the base and good plugins.",
        },
        {
          label: "AAC decoder",
          value: p.decoder || "None",
          status: p.decoder ? "ok" : "error",
          detail: p.decoder
            ? undefined
            : "Install gst-libav or the fdkaac plugin to decode Apple Music's AAC.",
        },
        {
          label: "Audio output",
          value: p.sink || "None",
          status: p.sink ? "ok" : "warning",
          detail: p.sink
            ? undefined
            : "No PipeWire or PulseAudio sink was found; GStreamer falls back to autoaudiosink.",
        },
        {
          label: "Playback host",
          value: p.hostRunning
            ? `Running · pid ${p.hostPid}`
            : "Stopped (starts with the next song)",
        },
      ],
    },
    {
      title: "Credentials",
      rows: [
        d.keyring.unencrypted
          ? {
              label: "Token storage",
              value: d.keyring.backend,
              status: "warning",
              detail:
                "Your Apple Music token is in a file only your user can read, not in a keyring. Start GNOME Keyring or KWallet and Sona moves it there.",
            }
          : { label: "Token storage", value: d.keyring.backend || dash, status: "ok" },
      ],
    },
    {
      title: "Library database",
      rows: [
        { label: "File", value: d.database.path },
        { label: "Size", value: formatBytes(d.database.sizeBytes) },
        { label: "Schema version", value: String(d.database.schemaVersion) },
        {
          label: "Contents",
          value: [
            plural(d.database.counts.songs, "song"),
            plural(d.database.counts.albums, "album"),
            plural(d.database.counts.artists, "artist"),
            plural(d.database.counts.playlists, "playlist"),
          ].join(", "),
        },
        {
          label: "Last full sync",
          value: d.database.lastSync ? relativeTime(d.database.lastSync) : "Never",
        },
        { label: "Artwork cache", value: formatBytes(d.artworkCacheBytes) },
      ],
    },
    {
      title: "Rendering",
      rows: [
        {
          label: "WebKitGTK",
          value: d.webkit.version || "Unknown",
          status: d.webkit.version ? "ok" : "warning",
        },
        { label: "Session", value: d.app.session || dash },
        ...["WEBKIT_DISABLE_DMABUF_RENDERER", "WEBKIT_DISABLE_COMPOSITING_MODE"].map((name) => {
          const e = env(name);
          return { label: name, value: e?.set ? `${e.value}` : "not set" };
        }),
      ],
    },
    {
      title: "Sona",
      rows: [
        { label: "Version", value: d.app.version || dash },
        { label: "Go", value: d.app.goVersion },
        { label: "Platform", value: d.app.platform },
        {
          label: "Start-up to first paint",
          value: d.app.startupMs ? `${d.app.startupMs} ms` : dash,
        },
        { label: "Log file", value: d.paths.logFile },
        { label: "Settings", value: d.paths.settings },
        { label: "Data", value: d.paths.data },
        { label: "Cache", value: d.paths.cache },
      ],
    },
  ];
}

/** Plain text for bug reports: one "label: value" line per row. */
function diagnosticsReport(groups: Group[]): string {
  return groups
    .map(
      (g) =>
        `## ${g.title}\n` +
        g.rows
          .map(
            (r) =>
              `${r.label}: ${r.value}${r.status && r.status !== "info" ? ` [${r.status}]` : ""}`,
          )
          .join("\n"),
    )
    .join("\n\n");
}

function StatusIcon({ status }: { status?: Status }) {
  if (status === "ok")
    return (
      <CircleCheck aria-label="OK" className="size-4 shrink-0 text-positive" strokeWidth={1.75} />
    );
  if (status === "warning")
    return (
      <TriangleAlert aria-label="Warning" className="size-4 shrink-0 text-ink" strokeWidth={1.75} />
    );
  if (status === "error")
    return (
      <CircleAlert
        aria-label="Problem"
        className="size-4 shrink-0 text-danger"
        strokeWidth={1.75}
      />
    );
  return <span aria-hidden className="size-4 shrink-0" />;
}

function RenderingToggles() {
  const settings = useSettings();
  const update = useUpdateSettings();
  const s = settings.data;
  if (!s) return null;
  const set = (patch: Partial<typeof s.diagnostics>) =>
    update.mutate({ ...s, diagnostics: { ...s.diagnostics, ...patch } });
  return (
    <div className="mt-3 rounded-lg bg-surface px-5">
      {(
        [
          [
            "disableDmabufRenderer",
            "Disable the DMA-BUF renderer",
            "Sets WEBKIT_DISABLE_DMABUF_RENDERER=1. Try it if the window is blank or flickers on your GPU driver.",
          ],
          [
            "disableCompositingMode",
            "Disable compositing",
            "Sets WEBKIT_DISABLE_COMPOSITING_MODE=1: software rendering. Turn on Reduce effects with it.",
          ],
        ] as const
      ).map(([key, title, desc]) => (
        <div
          key={key}
          className="flex items-center justify-between gap-8 border-b border-line py-4 last:border-b-0"
        >
          <div>
            <label htmlFor={key} className="text-ui font-medium text-ink">
              {title}
            </label>
            <p id={`${key}-desc`} className="mt-0.5 max-w-[56ch] text-sub text-ink-muted">
              {desc} Takes effect after you restart Sona.
            </p>
          </div>
          <Switch
            id={key}
            aria-describedby={`${key}-desc`}
            checked={s.diagnostics[key]}
            onCheckedChange={(on) => set({ [key]: on })}
          />
        </div>
      ))}
    </div>
  );
}

export function DiagnosticsSection() {
  const q = useQuery({
    queryKey: keys.diagnostics,
    queryFn: () => SystemService.Diagnostics(),
    staleTime: 0,
  });
  const groups = q.data ? diagnosticsGroups(q.data) : [];
  const problems = groups.flatMap((g) => g.rows).filter((r) => r.status === "error").length;

  const copy = () =>
    Clipboard.SetText(diagnosticsReport(groups)).then(
      () => toast("Diagnostics copied"),
      (err) => toast.error("Couldn't copy", { description: callFailure(err).message }),
    );
  const clearCache = () =>
    SystemService.ClearCache().then(
      (r) => {
        toast(
          `Cleared ${formatBytes(r.artworkBytes)} of artwork and ${plural(r.catalogEntries, "cached page")}`,
        );
        void q.refetch();
      },
      (err) => toast.error("Couldn't clear the cache", { description: callFailure(err).message }),
    );

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <p className="mr-auto text-sub text-ink-muted" aria-live="polite">
          {q.isFetching
            ? "Checking the playback host, database and keyring…"
            : q.data
              ? problems
                ? `${plural(problems, "problem")} found`
                : "Everything Sona needs is working"
              : ""}
        </p>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void q.refetch()}
          disabled={q.isFetching}
        >
          <RotateCw strokeWidth={1.75} />
          Check again
        </Button>
        <Button variant="outline" size="sm" onClick={() => void copy()} disabled={!q.data}>
          <Copy strokeWidth={1.75} />
          Copy report
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void run("Couldn't open the log folder", SystemService.OpenLogFolder())}
        >
          <FolderOpen strokeWidth={1.75} />
          Open log folder
        </Button>
      </div>

      {q.error && (
        <ErrorBlock
          title="Couldn't run the checks"
          error={q.error}
          onRetry={() => void q.refetch()}
          retrying={q.isRefetching}
        />
      )}
      {q.isPending && (
        <div aria-busy className="rounded-lg bg-surface px-5 py-2">
          {Array.from({ length: 7 }, (_, i) => (
            <div key={i} className="flex h-10 items-center gap-6">
              <div className="h-3 w-32 rounded-sm bg-hover" />
              <div
                className="h-3 rounded-sm bg-hover"
                style={{ width: `${20 + ((i * 13) % 35)}%` }}
              />
            </div>
          ))}
        </div>
      )}

      {groups.map((g) => (
        <section key={g.title} className="mt-6 first:mt-0">
          <h3 className="mb-2 text-ui font-semibold text-ink">{g.title}</h3>
          <dl className="rounded-lg bg-surface px-5 py-1.5">
            {g.rows.map((r) => (
              <div key={r.label} className="grid grid-cols-[200px_minmax(0,1fr)] gap-4 py-2">
                <dt className="text-sub text-ink-muted">{r.label}</dt>
                <dd className="min-w-0">
                  <span className="flex items-start gap-2">
                    <StatusIcon status={r.status} />
                    <span
                      className={cn(
                        "min-w-0 text-sub break-words text-ink",
                        r.status === "error" && "font-medium",
                      )}
                    >
                      {r.value}
                    </span>
                  </span>
                  {r.detail && <p className="mt-1 ml-6 text-sub text-ink-muted">{r.detail}</p>}
                </dd>
              </div>
            ))}
          </dl>
          {g.title === "Rendering" && <RenderingToggles />}
          {g.title === "Library database" && (
            <Button variant="ghost" size="sm" className="mt-2" onClick={() => void clearCache()}>
              Clear artwork and catalogue cache
            </Button>
          )}
        </section>
      ))}
    </div>
  );
}
