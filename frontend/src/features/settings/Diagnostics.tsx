import { plural, t } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { SystemService, type CDMStatus, type Diagnostics } from "@bindings/services";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Clipboard } from "@wailsio/runtime";
import { CircleAlert, CircleCheck, Copy, FolderOpen, RotateCw, TriangleAlert } from "lucide-react";
import { toast } from "sonner";

import { cdmFailedState, cdmPercent, cdmProblem, useCdm } from "@/features/cdm";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { Button } from "@/shared/ui/button";
import { Switch } from "@/shared/ui/switch";
import { run } from "@/shared/lib/run";
import { useCapabilities } from "@/shared/lib/capabilities";
import { callFailure } from "@/shared/lib/errors";
import { formatBytes, relativeTime } from "@/shared/lib/format";
import { keys } from "@/shared/lib/keys";
import { cn } from "@/shared/lib/utils";

import { useSettings, useUpdateSettings } from "./queries";
import { UpdatesGroup } from "./Updates";

type Status = "ok" | "warning" | "error" | "info";

interface Row {
  label: string;
  value: string;
  status?: Status;
  /** What to do about a warning or error, or what the value means. */
  detail?: string;
}

interface Group {
  title: string;
  rows: Row[];
}

const dash = "—";

const NETNS_NOTE = "network namespace";

/** WebKitGTK rendering switches, shown by their environment variable names. */
// eslint-disable-next-line lingui/no-unlocalized-strings -- environment variable names, not prose
const WEBKIT_ENV = ["WEBKIT_DISABLE_DMABUF_RENDERER", "WEBKIT_DISABLE_COMPOSITING_MODE"] as const;

/**
 * The playback host's confinement (cdmhost.Sandbox): namespaces, Landlock
 * and seccomp on Linux; macOS's sandbox-exec profile, or none, on macOS.
 */
function sandboxRow(p: Diagnostics["playback"]): Row {
  const label = t`Host sandbox`;
  if (p.sandboxed)
    return p.sandboxKind === "seatbelt"
      ? { label, value: t`Sandboxed by macOS · no network or keychain access`, status: "ok" }
      : { label, value: t`Sandboxed · no network or session access`, status: "ok" };
  if (!p.sandboxNote) return { label, value: t`Unknown (the host did not start)` };
  const note = p.sandboxNote;
  if (p.cdmBlocked) return { label, value: t`Not sandboxed`, status: "error", detail: note };
  if (p.sandboxKind === "none")
    return {
      label,
      value: t`Not sandboxed`,
      status: "warning",
      detail: t`${note}. Playback still works, but the host can reach the network and your files. macOS's sandbox-exec confines it where it works.`,
    };
  return {
    label,
    value: t`Not fully sandboxed`,
    status: "warning",
    // Go's note names the missing network namespace when user namespaces are blocked.
    detail: note.includes(NETNS_NOTE)
      ? t`${note}. Playback still works. To sandbox it fully, allow unprivileged user namespaces (sysctl kernel.unprivileged_userns_clone, or the AppArmor user namespace restriction).`
      : t`${note}. Playback still works.`,
  };
}

/**
 * The scheduling the playing host's audio thread got (cdmhost.Realtime).
 * The sandboxed host has no D-Bus, so on Linux Sona asks RTKit or the
 * Realtime portal for it on the host's behalf once its output plays.
 */
function realtimeRow(p: Diagnostics["playback"]): Row {
  const label = t`Audio priority`;
  const priority = p.realtimePriority;
  const note = p.realtimeNote;
  const via = p.realtimeVia === "portal" ? t`Realtime portal` : p.realtimeVia;
  const skips = t`Audio may skip while the computer is under heavy load.`;
  switch (p.realtime) {
    case "granted":
      return { label, value: t`Realtime · priority ${priority} (${via})`, status: "ok" };
    case "raised":
      return {
        label,
        value: t`Raised · nice ${priority} (${via})`,
        status: "ok",
        detail: t`Realtime priority was refused: ${note}.`,
      };
    case "denied":
      return { label, value: t`Normal`, status: "warning", detail: t`${note}. ${skips}` };
    case "unavailable":
      return {
        label,
        value: t`Normal`,
        status: "warning",
        detail: t`Neither RTKit nor the Realtime portal is available (${note}). ${skips} Install and enable RTKit (rtkit) to fix this.`,
      };
    case "unsupported":
      return { label, value: t`Not requested on this platform` };
    default:
      return {
        label,
        value: p.hostRunning ? t`Requested when audio starts` : t`Requested when playback starts`,
      };
  }
}

/** Token storage backends Go reports, by name; any other keeps Go's name. */
function keyringLabel(backend: string): string {
  switch (backend) {
    case "secret-service":
      return t`Secret Service keyring`;
    case "keychain":
      return t`macOS Keychain`;
    case "credential-manager":
      return t`Windows Credential Manager`;
    default:
      return backend || dash;
  }
}

/** The Widevine CDM's verdict: the probe's when it loaded one, else the download's state. */
function cdmStateRow(p: Diagnostics["playback"], cdm: CDMStatus): Row {
  const label = t`Widevine CDM`;
  if (p.cdmOk) return { label, value: t`Working`, status: "ok" };
  if (p.cdmBlocked) {
    return {
      label,
      value: t`Blocked`,
      status: "error",
      detail: t`Sona loads the CDM only inside its sandbox, which keeps it off the network, and the sandbox could not be set up. Only 30-second previews play.`,
    };
  }
  switch (cdm.state) {
    case "idle":
      return {
        label,
        value: t`Not downloaded yet`,
        detail: t`Sona downloads it from Google once it's online. Songs play when it's ready.`,
      };
    case "downloading": {
      const percent = cdmPercent(cdm);
      return {
        label,
        value: percent ? t`Downloading · ${percent}` : t`Downloading…`,
        detail: t`Songs play when the download finishes.`,
      };
    }
    case "failed":
      return {
        label,
        value: cdmFailedState(cdm.code),
        status: "error",
        detail: cdmProblem(cdm.code),
      };
  }
  return { label, value: t`Doesn't start`, status: "error", detail: p.cdmError || undefined };
}

/** Where the CDM comes from and how its updates are going. */
function cdmSourceRows(cdm: CDMStatus): Row[] {
  if (cdm.state === "") return [];
  const checked = cdm.checkedAt ? relativeTime(cdm.checkedAt) : "";
  let updates: Row;
  if (cdm.state === "ready" && cdm.updating) {
    const percent = cdmPercent(cdm);
    updates = {
      label: t`Updates`,
      value: percent ? t`Downloading an update · ${percent}` : t`Downloading an update`,
      detail: t`It applies the next time playback starts.`,
    };
  } else if (cdm.state === "ready" && cdm.code) {
    updates = {
      label: t`Updates`,
      value: t`Couldn't check for updates`,
      status: "warning",
      detail: cdmProblem(cdm.code),
    };
  } else {
    updates = { label: t`Updates`, value: checked ? t`Checked ${checked}` : t`Not checked yet` };
  }
  return [{ label: t`Source`, value: t`Downloaded from Google` }, updates];
}

/**
 * The CDM playback engine's checks: the CDM, GStreamer, the host and its
 * sandbox. The probe child that checks GStreamer and the sandbox needs a
 * CDM to load, so without one (the report's CDM not ready) those rows say
 * they weren't checked rather than failing.
 */
function cdmRows(p: Diagnostics["playback"], cdm: CDMStatus): Row[] {
  const { cdmInterface, hostInterface, hostPid, audioQuality } = p;
  const probed = p.cdm.state === "ready" || p.cdmOk || p.cdmBlocked;
  const unchecked = t`Checked once Widevine is ready`;
  return [
    cdmStateRow(p, cdm),
    ...cdmSourceRows(cdm),
    { label: t`Version`, value: cdm.version || p.cdmVersion || dash },
    { label: t`CDM file`, value: p.cdmPath || cdm.path || dash },
    {
      label: t`CDM interface`,
      value: cdmInterface ? t`${cdmInterface} (host ${hostInterface})` : dash,
    },
    !probed
      ? { label: t`GStreamer`, value: unchecked }
      : {
          label: t`GStreamer`,
          value: p.gstreamer || t`Not found`,
          status: p.gstreamer ? "ok" : "error",
          detail: p.gstreamer
            ? undefined
            : t`Install GStreamer 1.x with the base and good plugins.`,
        },
    !probed
      ? { label: t`AAC decoder`, value: unchecked }
      : {
          label: t`AAC decoder`,
          value: p.decoder || t`None`,
          status: p.decoder ? "ok" : "error",
          detail: p.decoder
            ? undefined
            : t`Install gst-libav or the fdkaac plugin to decode Apple Music's AAC.`,
        },
    !probed
      ? { label: t`Audio output`, value: unchecked }
      : {
          label: t`Audio output`,
          value: p.sink || t`None`,
          status: p.sink ? "ok" : "warning",
          detail: p.sink
            ? undefined
            : t`No PipeWire or PulseAudio sink was found; GStreamer falls back to autoaudiosink.`,
        },
    {
      label: t`Playback host`,
      value: p.hostRunning ? t`Running · pid ${hostPid}` : t`Stopped (starts with the next song)`,
    },
    sandboxRow(p),
    realtimeRow(p),
    {
      label: t`Stream format`,
      value: t`AAC ${audioQuality} kbps`,
      detail: t`Sona plays AAC only. Albums marked Lossless or Dolby Atmos play as AAC: Apple doesn't license lossless to desktop Widevine players.`,
    },
  ];
}

/**
 * The Diagnostics struct as labelled rows with a verdict where one applies.
 * The WebKitGTK rows only where the web view is WebKitGTK. `liveCdm` is the
 * CDM's status from cdm:status, newer than the report's copy while a
 * download runs.
 */
function diagnosticsGroups(d: Diagnostics, webkitGtk: boolean, liveCdm: CDMStatus | null): Group[] {
  const p = d.playback;
  const env = (name: string) => d.env.find((e) => e.name === name);
  const { songs, albums, artists, playlists } = d.database.counts;
  const { startupMs } = d.app;
  return [
    {
      title: t`Playback`,
      // Engine "" means this platform has no playback engine, so no probe ran.
      rows: p.engine
        ? cdmRows(p, liveCdm ?? p.cdm)
        : [{ label: t`Playback`, value: t`Not available on this platform yet` }],
    },
    {
      title: t`Credentials`,
      rows: [
        d.keyring.unencrypted
          ? {
              label: t`Token storage`,
              value: t`File (unencrypted)`,
              status: "warning",
              detail: t`Your Apple Music token is in a file only your user can read, not in a keyring. Start GNOME Keyring or KWallet and Sona moves it there.`,
            }
          : {
              label: t`Token storage`,
              value: keyringLabel(d.keyring.backend),
              status: "ok",
            },
      ],
    },
    {
      title: t`Library database`,
      rows: [
        { label: t`File`, value: d.database.path },
        { label: t`Size`, value: formatBytes(d.database.sizeBytes) },
        { label: t`Schema version`, value: String(d.database.schemaVersion) },
        {
          label: t`Contents`,
          value: [
            plural(songs, { one: "# song", other: "# songs" }),
            plural(albums, { one: "# album", other: "# albums" }),
            plural(artists, { one: "# artist", other: "# artists" }),
            plural(playlists, { one: "# playlist", other: "# playlists" }),
          ].join(", "),
        },
        {
          label: t`Last full sync`,
          value: d.database.lastSync ? relativeTime(d.database.lastSync) : t`Never`,
        },
        { label: t`Artwork cache`, value: formatBytes(d.artworkCacheBytes) },
      ],
    },
    // The renderer and its session are WebKitGTK's (Linux).
    ...(webkitGtk
      ? [
          {
            title: t`Rendering`,
            rows: [
              {
                label: t`WebKitGTK`,
                value: d.webkit.version || t`Unknown`,
                status: d.webkit.version ? ("ok" as const) : ("warning" as const),
              },
              { label: t`Session`, value: d.app.session || dash },
              ...WEBKIT_ENV.map((name) => {
                const e = env(name);
                return { label: name, value: e?.set ? `${e.value}` : t`not set` };
              }),
            ],
          },
        ]
      : []),
    {
      title: t`Sona`,
      rows: [
        { label: t`Version`, value: d.app.version || dash },
        { label: t`Go`, value: d.app.goVersion },
        { label: t`Platform`, value: d.app.platform },
        {
          label: t`Start-up to first paint`,
          value: startupMs ? t`${startupMs} ms` : dash,
        },
        { label: t`Log file`, value: d.paths.logFile },
        { label: t`Settings`, value: d.paths.settings },
        { label: t`Data`, value: d.paths.data },
        { label: t`Cache`, value: d.paths.cache },
      ],
    },
  ];
}

function StatusIcon({ status }: { status?: Status }) {
  const { t } = useLingui();
  if (status === "ok")
    return (
      <CircleCheck
        aria-label={t`OK`}
        className="size-4 shrink-0 text-positive"
        strokeWidth={1.75}
      />
    );
  if (status === "warning")
    return (
      <TriangleAlert
        aria-label={t`Warning`}
        className="size-4 shrink-0 text-ink"
        strokeWidth={1.75}
      />
    );
  if (status === "error")
    return (
      <CircleAlert
        aria-label={t`Problem`}
        className="size-4 shrink-0 text-danger"
        strokeWidth={1.75}
      />
    );
  return <span aria-hidden className="size-4 shrink-0" />;
}

function RenderingToggles() {
  const { t } = useLingui();
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
            t`Disable the DMA-BUF renderer`,
            t`Sets WEBKIT_DISABLE_DMABUF_RENDERER=1. Try it if the window is blank or flickers on your GPU driver. Takes effect after you restart Sona.`,
          ],
          [
            "disableCompositingMode",
            t`Disable compositing`,
            t`Sets WEBKIT_DISABLE_COMPOSITING_MODE=1: software rendering. Turn on Reduce effects with it. Takes effect after you restart Sona.`,
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
              {desc}
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
  const { t } = useLingui();
  const q = useQuery({
    queryKey: keys.diagnostics,
    queryFn: () => SystemService.Diagnostics(),
    staleTime: 0,
  });
  const webkitGtk = useCapabilities()?.webkitGtk ?? false;
  const liveCdm = useCdm((s) => s.status);
  const groups = q.data ? diagnosticsGroups(q.data, webkitGtk, liveCdm) : [];
  const problems = groups.flatMap((g) => g.rows).filter((r) => r.status === "error").length;

  // The redacted bug-report bundle: Go gathers the report, settings, crash
  // output and recent log lines, scrubs secrets and saves a copy.
  const bundle = useMutation({
    mutationFn: async () => {
      const b = await SystemService.DiagnosticsBundle();
      await Clipboard.SetText(b.text);
      return b;
    },
    onSuccess: (b) => {
      const file = b.path.split("/").pop();
      toast(t`Diagnostics copied`, {
        description: b.path
          ? t`Tokens and your e-mail address are removed. A copy is saved as ${file} in the log folder.`
          : t`Tokens and your e-mail address are removed.`,
        action: b.path
          ? {
              label: t`Show`,
              onClick: () =>
                void run(t`Couldn't open the log folder`, SystemService.OpenLogFolder()),
            }
          : undefined,
      });
    },
    onError: (err) =>
      toast.error(t`Couldn't collect diagnostics`, { description: callFailure(err).message }),
  });

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <p className="mr-auto text-sub text-ink-muted" aria-live="polite">
          {q.isFetching
            ? t`Checking the playback host, database and keyring…`
            : q.data
              ? problems
                ? t`${plural(problems, { one: "# problem found", other: "# problems found" })}`
                : t`Everything Sona needs is working`
              : ""}
        </p>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void q.refetch()}
          disabled={q.isFetching}
        >
          <RotateCw strokeWidth={1.75} />
          <Trans>Check again</Trans>
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={() => bundle.mutate()}
          disabled={bundle.isPending}
        >
          <Copy strokeWidth={1.75} />
          {bundle.isPending ? <Trans>Collecting…</Trans> : <Trans>Copy diagnostics</Trans>}
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void run(t`Couldn't open the log folder`, SystemService.OpenLogFolder())}
        >
          <FolderOpen strokeWidth={1.75} />
          <Trans>Open log folder</Trans>
        </Button>
      </div>

      {q.error && (
        <ErrorBlock
          title={t`Couldn't run the checks`}
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
          {webkitGtk && g.title === t`Rendering` && <RenderingToggles />}
        </section>
      ))}
      {q.data && <UpdatesGroup />}
    </div>
  );
}
