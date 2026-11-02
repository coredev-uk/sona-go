import { plural, t } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { RealtimeRefusal, SandboxGap } from "@bindings/playback/models";
import { MotionService, SystemService, type CDMStatus, type Diagnostics } from "@bindings/services";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Clipboard } from "@wailsio/runtime";
import { CircleAlert, CircleCheck, Copy, FolderOpen, RotateCw, TriangleAlert } from "lucide-react";
import { Fragment } from "react";
import { toast } from "sonner";

import { cdmFailedState, cdmPercent, cdmProblem, useCdm } from "@/features/cdm";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { Button } from "@/shared/ui/button";
import { Switch } from "@/shared/ui/switch";
import { run } from "@/shared/lib/run";
import { useCapabilities } from "@/shared/lib/capabilities";
import { callFailure, codeMessage } from "@/shared/lib/errors";
import { formatBytes, relativeTime } from "@/shared/lib/format";
import { keys } from "@/shared/lib/keys";
import { useMotionLog, useMotionState } from "@/shared/lib/motion";
import { cn } from "@/shared/lib/utils";

import { motionGroup, motionReport } from "./MotionDiagnostics";
import { useSettings, useUpdateSettings } from "./queries";
import { SettingRow, SettingsPanel } from "./SettingRow";

type Status = "ok" | "warning" | "error" | "info";

export interface Row {
  label: string;
  value: string;
  status?: Status;
  /** What to do about a warning or error, or what the value means. */
  detail?: string;
}

export interface Group {
  title: string;
  rows: Row[];
}

const dash = "—";

/** The variable that turns the macOS sandbox off (cdmhost.EnvSeatbelt). */
const SEATBELT_OFF = "SONA_SEATBELT=0";

/** What a missing layer of the host's sandbox means (playback.SandboxGap), as a sentence. */
function sandboxGap(gap: SandboxGap): string {
  switch (gap) {
    case SandboxGap.SandboxGapNamespaces:
      return t`It has no network or mount namespace of its own, because unprivileged user namespaces are blocked.`;
    case SandboxGap.SandboxGapFilesystem:
      return t`It has no private view of the filesystem.`;
    case SandboxGap.SandboxGapLandlock:
      return t`Landlock doesn't restrict its files.`;
    case SandboxGap.SandboxGapSeccomp:
      return t`Its seccomp filter isn't installed.`;
    case SandboxGap.SandboxGapNetwork:
      return t`It can open network connections.`;
    case SandboxGap.SandboxGapKeychain:
      return t`It can read your keychain folder.`;
    case SandboxGap.SandboxGapTurnedOff:
      return t`The macOS sandbox is turned off (${SEATBELT_OFF}).`;
    case SandboxGap.SandboxGapNoSeatbelt:
      return t`macOS's sandbox-exec is missing or didn't start.`;
    case SandboxGap.SandboxGapRefused:
      return t`macOS's sandbox-exec refused Sona's profile.`;
    case SandboxGap.$zero:
      return "";
  }
}

/** Why realtime priority was refused (playback.RealtimeRefusal), as a sentence. */
function realtimeRefusal(refusal: RealtimeRefusal, via: string): string {
  switch (refusal) {
    case RealtimeRefusal.RealtimeNoPriority:
      return t`${via} allows no realtime priority.`;
    case RealtimeRefusal.RealtimeCPULimit:
      return t`Sona couldn't limit the host's realtime CPU time, which ${via} requires.`;
    case RealtimeRefusal.RealtimeRefused:
      return t`${via} refused it.`;
    case RealtimeRefusal.$zero:
      return "";
  }
}

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
  const gaps = p.sandboxGaps ?? [];
  if (!p.sandboxKind || gaps.length === 0)
    return { label, value: t`Unknown (the host did not start)` };
  const note = gaps.map(sandboxGap).join(" ");
  if (p.cdmBlocked) return { label, value: t`Not sandboxed`, status: "error", detail: note };
  if (p.sandboxKind === "none")
    return {
      label,
      value: t`Not sandboxed`,
      status: "warning",
      detail: t`${note} Playback still works, but the host can reach the network and your files. macOS's sandbox-exec confines it where it works.`,
    };
  return {
    label,
    value: t`Not fully sandboxed`,
    status: "warning",
    detail: gaps.includes(SandboxGap.SandboxGapNamespaces)
      ? t`${note} Playback still works. To sandbox it fully, allow unprivileged user namespaces (sysctl kernel.unprivileged_userns_clone, or the AppArmor user namespace restriction).`
      : t`${note} Playback still works.`,
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
  const via = p.realtimeVia === "portal" ? t`Realtime portal` : p.realtimeVia;
  const refusal = realtimeRefusal(p.realtimeRefusal, via);
  const skips = t`Audio may skip while the computer is under heavy load.`;
  switch (p.realtime) {
    case "granted":
      return { label, value: t`Realtime · priority ${priority} (${via})`, status: "ok" };
    case "raised":
      return {
        label,
        value: t`Raised · nice ${priority} (${via})`,
        status: "ok",
        detail: refusal
          ? t`Realtime priority was refused: ${refusal}`
          : t`Realtime priority was refused.`,
      };
    case "denied":
      return {
        label,
        value: t`Normal`,
        status: "warning",
        detail: refusal ? `${refusal} ${skips}` : skips,
      };
    case "unavailable":
      return {
        label,
        value: t`Normal`,
        status: "warning",
        detail: t`Neither RTKit nor the Realtime portal is available. ${skips} Install and enable RTKit (rtkit) to fix this.`,
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
  return {
    label,
    value: t`Doesn't start`,
    status: "error",
    detail: p.cdmError ? codeMessage(p.cdmError) : undefined,
  };
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
    <SettingsPanel className="mt-3">
      {(
        [
          [
            "disableDmabufRenderer",
            t`Disable the DMA-BUF renderer`,
            t`Try it if the window stays blank or flickers on your graphics driver. Applies from Sona's next start (WEBKIT_DISABLE_DMABUF_RENDERER=1).`,
          ],
          [
            "disableCompositingMode",
            t`Disable compositing`,
            t`Software rendering, for when the DMA-BUF switch isn't enough. Turn on Reduce effects in Appearance with it. Applies from Sona's next start (WEBKIT_DISABLE_COMPOSITING_MODE=1).`,
          ],
        ] as const
      ).map(([key, title, desc]) => (
        <SettingRow key={key} id={key} title={title} description={desc}>
          <Switch
            id={key}
            aria-describedby={`${key}-desc`}
            checked={s.diagnostics[key]}
            onCheckedChange={(on) => set({ [key]: on })}
          />
        </SettingRow>
      ))}
    </SettingsPanel>
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
  const motionState = useMotionState();
  const motionLog = useMotionLog();
  const motionStatus = useQuery({
    queryKey: [...keys.diagnostics, "motion"],
    queryFn: () => MotionService.Status(),
    staleTime: 0,
    // Downloads happen while the page is open (another window's covers).
    refetchInterval: 5000,
  });
  const visible = document.visibilityState === "visible";
  const motion = motionGroup(motionState, motionLog.last, motionStatus.data, visible);
  const groups = q.data ? diagnosticsGroups(q.data, webkitGtk, liveCdm) : [];
  // Motion artwork after Playback.
  if (motion && groups.length > 0) groups.splice(1, 0, motion);
  const rows = groups.flatMap((g) => g.rows);
  const problems = rows.filter((r) => r.status === "error").length;
  const warnings = rows.filter((r) => r.status === "warning").length;

  // The redacted bug-report bundle: Go gathers the report, settings, crash
  // output and recent log lines, scrubs secrets and saves a copy.
  const bundle = useMutation({
    mutationFn: async () => {
      // What only the web view knows about motion artwork goes in too.
      await MotionService.Report(
        motionReport(motionState, motionLog.last, motionLog.playing.length, visible),
      ).catch(() => {});
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
      <div className="mb-8 flex flex-wrap items-start gap-x-6 gap-y-3 border-t border-line pt-4">
        <div className="mr-auto flex min-h-8 items-start gap-2.5" aria-live="polite">
          {q.data && !q.isFetching && (
            <span className="mt-0.5">
              <StatusIcon status={problems ? "error" : "ok"} />
            </span>
          )}
          <div>
            <p className="text-heading text-ink">
              {q.isFetching
                ? t`Checking the playback host, database and keyring…`
                : q.data
                  ? problems
                    ? t`${plural(problems, { one: "# problem found", other: "# problems found" })}`
                    : t`Everything Sona needs is working`
                  : ""}
            </p>
            {q.data && !q.isFetching && warnings > 0 && (
              <p className="text-sub text-ink-muted">
                {plural(warnings, {
                  one: "# warning to look at below",
                  other: "# warnings to look at below",
                })}
              </p>
            )}
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
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
            data-setting="diagnostics-copy"
            variant="outline"
            size="sm"
            onClick={() => bundle.mutate()}
            disabled={bundle.isPending}
          >
            <Copy strokeWidth={1.75} />
            {bundle.isPending ? <Trans>Collecting…</Trans> : <Trans>Copy diagnostics</Trans>}
          </Button>
          <Button
            data-setting="diagnostics-logs"
            variant="outline"
            size="sm"
            onClick={() => void run(t`Couldn't open the log folder`, SystemService.OpenLogFolder())}
          >
            <FolderOpen strokeWidth={1.75} />
            <Trans>Open log folder</Trans>
          </Button>
        </div>
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
        <div aria-busy>
          <div className="mb-3 h-4 w-28 rounded-sm bg-hover" />
          {Array.from({ length: 7 }, (_, i) => (
            <div key={i} className="flex h-10 items-center gap-6 border-t border-line">
              <div className="h-3 w-32 rounded-sm bg-hover" />
              <div
                className="h-3 rounded-sm bg-hover"
                style={{ width: `${20 + ((i * 13) % 35)}%` }}
              />
            </div>
          ))}
        </div>
      )}

      <div className="flex flex-col gap-10">
        {groups.map((g) => (
          <section key={g.title}>
            <h3 className="mb-3 text-heading text-ink">{g.title}</h3>
            <dl>
              {g.rows.map((r) => (
                <div
                  key={r.label}
                  className="grid grid-cols-[200px_minmax(0,1fr)] gap-6 border-t border-line py-2.5 max-[960px]:grid-cols-[160px_minmax(0,1fr)]"
                >
                  <dt className="text-sub [overflow-wrap:anywhere] text-ink-muted">
                    {/* Long variable names break after an underscore, not mid-word. */}
                    {r.label.split("_").map((part, i) => (
                      <Fragment key={i}>
                        {i > 0 && (
                          <>
                            _<wbr />
                          </>
                        )}
                        {part}
                      </Fragment>
                    ))}
                  </dt>
                  <dd className="min-w-0">
                    <span className="flex items-start gap-2">
                      <StatusIcon status={r.status} />
                      <span
                        className={cn(
                          "min-w-0 text-sub [overflow-wrap:anywhere] text-ink",
                          r.status === "error" && "font-medium",
                        )}
                      >
                        {r.value}
                      </span>
                    </span>
                    {r.detail && (
                      <p className="mt-1 ml-6 max-w-[64ch] text-sub text-ink-muted">{r.detail}</p>
                    )}
                  </dd>
                </div>
              ))}
            </dl>
            {webkitGtk && g.title === t`Rendering` && <RenderingToggles />}
          </section>
        ))}
      </div>
    </div>
  );
}
