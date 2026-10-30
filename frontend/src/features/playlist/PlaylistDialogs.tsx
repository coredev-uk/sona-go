import { PlaylistsService, type TreePlaylist } from "@bindings/services";
import { useNavigate } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { Check, FolderClosed, ListMusic, Plus, Search } from "lucide-react";
import { useMemo, useState, type FormEvent, type KeyboardEvent, type ReactNode } from "react";
import { toast } from "sonner";

import { Artwork } from "@/shared/components/Artwork";
import { callFailure } from "@/shared/lib/errors";
import { run } from "@/shared/lib/run";
import { Button } from "@/shared/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/shared/ui/dialog";
import { Input } from "@/shared/ui/input";
import { Switch } from "@/shared/ui/switch";

import { showDialog, useDialogs, type PlaylistDialog } from "./dialogs";
import { playlistKeys, setDetail, usePlaylistTree, useRecentTargets } from "./queries";
import { addTargets, buildTree, folderPaths } from "./tree";

type Of<K extends PlaylistDialog["kind"]> = Extract<PlaylistDialog, { kind: K }>;

/** Every playlist dialog, mounted once around the shell (a Provider slot). */
export function PlaylistDialogsProvider({ children }: { children: ReactNode }) {
  return (
    <>
      {children}
      <PlaylistDialogs />
    </>
  );
}

function PlaylistDialogs() {
  const open = useDialogs((s) => s.open);
  const close = useDialogs((s) => s.close);
  return (
    <Dialog open={!!open} onOpenChange={(o) => !o && close()}>
      {open?.kind === "add" && <AddToPlaylist d={open} />}
      {open?.kind === "create" && <PlaylistForm d={open} />}
      {open?.kind === "details" && <PlaylistForm d={open} />}
      {open?.kind === "folder" && <FolderForm d={open} />}
      {open?.kind === "move" && <MoveToFolder d={open} />}
    </Dialog>
  );
}

/** Up/Down move focus between the list's options; Home/End jump. */
function listKeys(e: KeyboardEvent<HTMLElement>) {
  const options = [...e.currentTarget.querySelectorAll<HTMLElement>("[data-option]")];
  const i = options.indexOf(document.activeElement as HTMLElement);
  const next =
    e.key === "ArrowDown"
      ? i + 1
      : e.key === "ArrowUp"
        ? i - 1
        : e.key === "Home"
          ? 0
          : e.key === "End"
            ? options.length - 1
            : null;
  if (next === null) return;
  e.preventDefault();
  options[Math.max(0, Math.min(options.length - 1, next))]?.focus();
}

function Option({
  icon,
  art,
  title,
  meta,
  current,
  onSelect,
}: {
  icon?: ReactNode;
  art?: string;
  title: string;
  meta?: string;
  current?: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      data-option
      onClick={onSelect}
      className="flex h-11 w-full items-center gap-3 rounded-md px-2 text-left transition-colors duration-100 hover:bg-hover focus-visible:bg-hover focus-visible:outline-offset-[-2px]"
    >
      {icon ? (
        <span className="grid size-8 shrink-0 place-items-center rounded-sm bg-hover text-ink-muted">
          {icon}
        </span>
      ) : (
        <Artwork src={art} size={32} alt="" />
      )}
      <span className="min-w-0 flex-1">
        <span className="block truncate text-ui text-ink">{title}</span>
        {meta && <span className="block truncate text-meta text-ink-faint num">{meta}</span>}
      </span>
      {current && (
        <Check aria-label="Current" className="size-4 text-ink-muted" strokeWidth={1.75} />
      )}
    </button>
  );
}

const songCount = (n: number) => (n === 1 ? "1 song" : `${n} songs`);

function AddToPlaylist({ d }: { d: Of<"add"> }) {
  const close = useDialogs((s) => s.close);
  const navigate = useNavigate();
  const qc = useQueryClient();
  const tree = usePlaylistTree();
  const recent = useRecentTargets(true);
  const [query, setQuery] = useState("");
  const nodes = useMemo(() => (tree.data ? buildTree(tree.data) : []), [tree.data]);
  const targets = useMemo(
    () => addTargets(nodes, recent.data ?? [], query),
    [nodes, recent.data, query],
  );

  const add = (p: TreePlaylist) => {
    close();
    const name = p.playlist.name;
    void PlaylistsService.Add(p.playlist.id, d.items).then(
      (detail) => {
        setDetail(qc, detail);
        void qc.invalidateQueries({ queryKey: playlistKeys.recentTargets });
        toast(`Added ${d.label} to “${name}”`, {
          action: {
            label: "Open",
            onClick: () => void navigate({ to: "/playlist/$id", params: { id: p.playlist.id } }),
          },
        });
      },
      (err) => toast.error(`Couldn't add to “${name}”`, { description: callFailure(err).message }),
    );
  };

  const row = (p: TreePlaylist) => (
    <Option
      key={p.playlist.id}
      art={p.playlist.art.small}
      title={p.playlist.name}
      meta={songCount(p.playlist.trackCount)}
      onSelect={() => add(p)}
    />
  );
  const empty = !targets.recent.length && !targets.groups.length;

  return (
    <DialogContent className="flex max-h-[min(640px,85vh)] max-w-md flex-col gap-0 p-0">
      <DialogHeader className="gap-1 px-5 pt-5 pb-3">
        <DialogTitle className="text-heading">Add to playlist</DialogTitle>
        <DialogDescription className="truncate text-sub text-ink-muted">
          {d.label}
        </DialogDescription>
      </DialogHeader>
      <div className="relative px-5 pb-3">
        <Search
          aria-hidden
          className="pointer-events-none absolute top-1/2 left-7.5 size-4 -translate-y-[calc(50%+6px)] text-ink-faint"
          strokeWidth={1.75}
        />
        <Input
          autoFocus
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") {
              e.preventDefault();
              e.currentTarget
                .closest("[role=dialog]")
                ?.querySelector<HTMLElement>("[data-option]")
                ?.focus();
            }
          }}
          placeholder="Find a playlist"
          aria-label="Find a playlist"
          className="h-8 pl-8"
        />
      </div>
      <div role="list" onKeyDown={listKeys} className="min-h-0 flex-1 overflow-y-auto px-3 pb-3">
        <Option
          icon={<Plus className="size-4" strokeWidth={1.75} />}
          title="New playlist…"
          onSelect={() =>
            showDialog({ kind: "create", items: d.items, label: d.label, folderId: "" })
          }
        />
        {targets.recent.length > 0 && (
          <section aria-label="Recent">
            <h3 className="px-2 pt-4 pb-1 text-meta text-ink-faint">Recent</h3>
            {targets.recent.map(row)}
          </section>
        )}
        {targets.groups.map((g) => (
          <section key={g.path || "top"} aria-label={g.path || "Playlists"}>
            <h3 className="flex items-center gap-1.5 px-2 pt-4 pb-1 text-meta text-ink-faint">
              {g.path && <FolderClosed aria-hidden className="size-3" strokeWidth={1.75} />}
              {g.path || "Playlists"}
            </h3>
            {g.items.map(row)}
          </section>
        ))}
        {empty && (
          <p className="px-2 pt-4 text-sub text-ink-muted">
            {query.trim()
              ? `No playlist you can edit is called “${query.trim()}”.`
              : tree.isPending
                ? "Loading your playlists…"
                : "You have no playlists you can edit yet."}
          </p>
        )}
      </div>
    </DialogContent>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-sub font-medium text-ink">{label}</span>
      {children}
    </label>
  );
}

function PlaylistForm({ d }: { d: Of<"create"> | Of<"details"> }) {
  const close = useDialogs((s) => s.close);
  const navigate = useNavigate();
  const qc = useQueryClient();
  const editing = d.kind === "details";
  const [name, setName] = useState(editing ? d.name : "");
  const [description, setDescription] = useState(editing ? d.description : "");
  const [isPublic, setPublic] = useState(editing ? d.isPublic : false);
  const [busy, setBusy] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim() || busy) return;
    setBusy(true);
    const input = { name, description, isPublic };
    if (d.kind === "details") {
      const ok = await run("Couldn't save the playlist", PlaylistsService.Update(d.id, input));
      setBusy(false);
      if (ok) {
        await qc.invalidateQueries({ queryKey: playlistKeys.detail(d.id) });
        close();
      }
      return;
    }
    try {
      const p = await PlaylistsService.Create(input, d.items, d.folderId);
      close();
      if (d.items.length) {
        toast(`Added ${d.label} to “${p.name}”`, {
          action: {
            label: "Open",
            onClick: () => void navigate({ to: "/playlist/$id", params: { id: p.id } }),
          },
        });
      } else {
        void navigate({ to: "/playlist/$id", params: { id: p.id } });
      }
    } catch (err) {
      toast.error("Couldn't create the playlist", { description: callFailure(err).message });
      setBusy(false);
    }
  };

  return (
    <DialogContent className="max-w-md">
      <form onSubmit={(e) => void submit(e)} className="flex flex-col gap-5">
        <DialogHeader className="gap-1">
          <DialogTitle className="text-heading">
            {editing ? "Edit playlist" : "New playlist"}
          </DialogTitle>
          <DialogDescription className="text-sub text-ink-muted">
            {editing
              ? "Changes appear in Apple Music on all your devices."
              : d.items.length
                ? `With ${d.label}.`
                : "Add songs from any album, playlist or search result."}
          </DialogDescription>
        </DialogHeader>
        <Field label="Name">
          <Input
            autoFocus
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={200}
            required
          />
        </Field>
        <Field label="Description">
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={3}
            maxLength={1000}
            className="min-h-16 w-full resize-none rounded-md border border-input bg-transparent px-3 py-2 text-ui outline-none placeholder:text-ink-faint focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30"
            placeholder="Optional"
          />
        </Field>
        <label className="flex items-center justify-between gap-4">
          <span>
            <span className="block text-sub font-medium text-ink">Show on my profile</span>
            <span className="block text-meta text-ink-faint">
              People who follow you can find and play it.
            </span>
          </span>
          <Switch checked={isPublic} onCheckedChange={setPublic} />
        </label>
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={close}>
            Cancel
          </Button>
          <Button type="submit" disabled={!name.trim() || busy}>
            {editing ? (busy ? "Saving…" : "Save") : busy ? "Creating…" : "Create playlist"}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  );
}

function FolderForm({ d }: { d: Of<"folder"> }) {
  const close = useDialogs((s) => s.close);
  const [name, setName] = useState(d.folder?.name ?? "");
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim() || busy) return;
    setBusy(true);
    const ok = await run(
      d.folder ? "Couldn't rename the folder" : "Couldn't create the folder",
      d.folder
        ? PlaylistsService.RenameFolder(d.folder.id, name)
        : PlaylistsService.CreateFolder(name),
    );
    setBusy(false);
    if (ok) close();
  };
  return (
    <DialogContent className="max-w-sm">
      <form onSubmit={(e) => void submit(e)} className="flex flex-col gap-5">
        <DialogHeader className="gap-1">
          <DialogTitle className="text-heading">
            {d.folder ? "Rename folder" : "New folder"}
          </DialogTitle>
          <DialogDescription className="text-sub text-ink-muted">
            Folders group playlists in the sidebar on all your devices.
          </DialogDescription>
        </DialogHeader>
        <Field label="Name">
          <Input
            autoFocus
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={200}
            required
          />
        </Field>
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={close}>
            Cancel
          </Button>
          <Button type="submit" disabled={!name.trim() || busy}>
            {d.folder ? "Rename" : "Create folder"}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  );
}

function MoveToFolder({ d }: { d: Of<"move"> }) {
  const close = useDialogs((s) => s.close);
  const tree = usePlaylistTree();
  const folders = useMemo(() => (tree.data ? folderPaths(buildTree(tree.data)) : []), [tree.data]);
  const move = (folderId: string, where: string) => {
    close();
    if (folderId === d.folderId) return;
    void run(`Couldn't move “${d.name}”`, PlaylistsService.MoveToFolder(d.id, folderId)).then(
      (ok) => ok && toast(`Moved “${d.name}” to ${where}`),
    );
  };
  return (
    <DialogContent className="flex max-h-[min(560px,85vh)] max-w-sm flex-col gap-0 p-0">
      <DialogHeader className="gap-1 px-5 pt-5 pb-3">
        <DialogTitle className="text-heading">Move to folder</DialogTitle>
        <DialogDescription className="truncate text-sub text-ink-muted">{d.name}</DialogDescription>
      </DialogHeader>
      <div role="list" onKeyDown={listKeys} className="min-h-0 flex-1 overflow-y-auto px-3 pb-3">
        <Option
          icon={<ListMusic className="size-4" strokeWidth={1.75} />}
          title="Top level"
          current={d.folderId === ""}
          onSelect={() => move("", "the top level")}
        />
        {folders.map(({ folder, path }) => (
          <Option
            key={folder.id}
            icon={<FolderClosed className="size-4" strokeWidth={1.75} />}
            title={path}
            current={folder.id === d.folderId}
            onSelect={() => move(folder.id, `“${folder.name}”`)}
          />
        ))}
        {!folders.length && !tree.isPending && (
          <p className="px-2 pt-3 text-sub text-ink-muted">
            You have no folders yet. Create one from the sidebar’s Playlists menu.
          </p>
        )}
      </div>
    </DialogContent>
  );
}
