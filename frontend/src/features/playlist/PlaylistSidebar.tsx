import {
  PlaylistsService,
  type Pin,
  type PlaylistFolder,
  type TreePlaylist,
} from "@bindings/services";
import { Plural, Trans, useLingui } from "@lingui/react/macro";
import { Link, useNavigate } from "@tanstack/react-router";
import {
  ChevronRight,
  Ellipsis,
  FolderClosed,
  FolderOpen,
  ListPlus,
  Pin as PinIcon,
  Plus,
} from "lucide-react";
import { memo, useMemo, useRef, useState, type ReactNode } from "react";

import { ActionMenu } from "@/shared/actions/ActionMenu";
import { Artwork } from "@/shared/components/Artwork";
import { useDrag, useDragActive } from "@/shared/dnd/drag";
import { run } from "@/shared/lib/run";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu";

import { deleteFolder, playPin, unpin } from "./commands";
import { showDialog } from "./dialogs";
import { addable, pinnable, useNewPlaylistDrop, usePinDrop, usePlaylistDrop } from "./drops";
import { usePins, usePlaylistTree } from "./queries";
import { sidebarPlaylists, type SidebarPlaylists, type TreeNode } from "./tree";

const ROW =
  "flex h-8 min-w-0 flex-1 items-center gap-2.5 rounded-md px-2.5 text-ui text-ink-muted transition-colors duration-100 hover:bg-hover hover:text-ink";
const ACTIVE =
  "data-[status=active]:bg-active data-[status=active]:font-medium data-[status=active]:text-ink";
/** A row a drag is over: inverted, so where the drop lands is unmistakable. */
const DROP_OVER =
  "bg-ink text-ground hover:bg-ink hover:text-ground data-[status=active]:bg-ink data-[status=active]:text-ground";
/** A drop zone shown only while a drag it takes is in progress. */
const DROP_ZONE =
  "flex h-8 items-center gap-2.5 rounded-md border border-dashed border-control-line px-2.5 text-ui text-ink-muted transition-colors duration-100";

function Heading({ children, action }: { children: string; action?: ReactNode }) {
  return (
    <div className="mt-6 mb-1.5 flex h-5 items-center justify-between px-2.5">
      <h2 className="text-meta text-ink-faint">{children}</h2>
      {action}
    </div>
  );
}

/** A row's quiet "more" trigger, shown while the row is hovered or focused. */
function MoreTrigger({ label }: { label: string }) {
  return (
    <DropdownMenuTrigger asChild>
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label={label}
        className="absolute top-1 right-1 text-ink-muted opacity-0 group-focus-within/row:opacity-100 group-hover/row:opacity-100 hover:text-ink data-[state=open]:opacity-100"
      >
        <Ellipsis />
      </Button>
    </DropdownMenuTrigger>
  );
}

/** Where a pin opens: its library page (a song opens its album). */
function pinLink(p: Pin) {
  switch (p.kind) {
    case "album":
      return { to: "/album/$id", params: { id: p.id } } as const;
    case "artist":
      return { to: "/artist/$id", params: { id: p.id } } as const;
    case "playlist":
      return { to: "/playlist/$id", params: { id: p.id } } as const;
    default:
      return p.albumId ? ({ to: "/album/$id", params: { id: p.albumId } } as const) : null;
  }
}

const PIN_ACTIONS = ["play", "shuffle", "drillIn"] as const;

/**
 * `upAfter`: the pin to place this one after to move it up ("" the top; null when first).
 * A pinned playlist the user can edit takes songs dropped on it.
 */
function PinRow({
  pin,
  upAfter,
  editable,
}: {
  pin: Pin;
  upAfter: string | null;
  editable: boolean;
}) {
  const { t } = useLingui();
  const actionLabels = { play: t`Play`, shuffle: t`Shuffle`, drillIn: t`Open` };
  const pinName = pin.name;
  const over = usePlaylistDrop(`pin:${pin.id}`, {
    id: pin.id,
    name: pin.name,
    canEdit: editable && pin.kind === "playlist",
  });
  const navigate = useNavigate();
  const link = pinLink(pin);
  const opens = pin.action === "drillIn" || (pin.kind === "artist" && !!link);
  const activate = () => {
    if (opens && link) void navigate(link);
    else void playPin(pin.id, pin.name, pin.action === "shuffle");
  };
  const art = (
    <Artwork
      src={pin.art.small}
      size={20}
      alt=""
      shape={pin.kind === "artist" ? "round" : "square"}
    />
  );
  return (
    <li className="group/row relative flex" data-drop={`pin:${pin.id}`}>
      {opens && link ? (
        <Link {...link} className={cn(ROW, ACTIVE, "pr-8", over && DROP_OVER)}>
          {art}
          <span className="truncate">{pin.name}</span>
        </Link>
      ) : (
        <button
          type="button"
          onClick={activate}
          className={cn(ROW, "pr-8 text-left", over && DROP_OVER)}
        >
          {art}
          <span className="truncate">{pin.name}</span>
          <span className="sr-only">{pin.action === "shuffle" ? t`, shuffles` : t`, plays`}</span>
        </button>
      )}
      <DropdownMenu>
        <MoreTrigger label={t`More for ${pinName}`} />
        <DropdownMenuContent align="start" className="w-48">
          <DropdownMenuLabel className="text-meta font-normal text-ink-faint">
            <Trans>When chosen</Trans>
          </DropdownMenuLabel>
          <DropdownMenuRadioGroup
            value={pin.action}
            onValueChange={(v) =>
              void run(t`Couldn't change the pin`, PlaylistsService.SetPinAction(pin.id, v))
            }
          >
            {PIN_ACTIONS.map((a) => (
              <DropdownMenuRadioItem key={a} value={a}>
                {actionLabels[a]}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
          <DropdownMenuSeparator />
          {link && !opens && (
            <DropdownMenuItem onSelect={() => void navigate(link)}>
              {pin.kind === "song" ? t`Go to album` : t`Open`}
            </DropdownMenuItem>
          )}
          {upAfter !== null && (
            <DropdownMenuItem
              onSelect={() =>
                void run(t`Couldn't move the pin`, PlaylistsService.MovePin(pin.id, upAfter))
              }
            >
              <Trans>Move up</Trans>
            </DropdownMenuItem>
          )}
          <DropdownMenuItem onSelect={() => void unpin(pin.id, pin.name)}>
            <Trans>Unpin</Trans>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </li>
  );
}

function Pins() {
  const { t } = useLingui();
  const pins = usePins();
  const tree = usePlaylistTree();
  const list = pins.data ?? [];
  const editable = useMemo(
    () => new Set(tree.data?.playlists.filter((p) => p.playlist.canEdit).map((p) => p.playlist.id)),
    [tree.data],
  );
  const over = usePinDrop(list);
  // The section appears while library items that can be pinned are dragged, even without pins.
  const offered = useDragActive((p) => pinnable(p, list).length > 0);
  if (!list.length && !offered) return null;
  return (
    // Unnamed: a heading inside the Main nav, not a landmark of its own.
    <section data-drop="pins">
      <Heading>{t`Pinned`}</Heading>
      <ul className="flex flex-col gap-0.5">
        {list.map((p, i) => (
          // Moving up one place puts the pin after the one two above it.
          <PinRow
            key={p.id}
            pin={p}
            upAfter={i === 0 ? null : i === 1 ? "" : list[i - 2].id}
            editable={editable.has(p.id)}
          />
        ))}
        {offered && (
          <li
            aria-hidden
            className={cn(DROP_ZONE, over && "border-transparent bg-ink text-ground")}
          >
            <PinIcon className="size-4 shrink-0" strokeWidth={1.75} />
            <Trans>Pin here</Trans>
          </li>
        )}
      </ul>
    </section>
  );
}

const OPEN_KEY = "sona.playlist-folders.open";

function readOpenFolders(): ReadonlySet<string> {
  try {
    return new Set(JSON.parse(localStorage.getItem(OPEN_KEY) ?? "[]") as string[]);
  } catch {
    return new Set();
  }
}

function useOpenFolders(): FolderState {
  const [open, setOpen] = useState(readOpenFolders);
  const toggle = (id: string) =>
    setOpen((prev) => {
      const next = new Set(prev);
      if (!next.delete(id)) next.add(id);
      localStorage.setItem(OPEN_KEY, JSON.stringify([...next]));
      return next;
    });
  return { open, toggle };
}

/** Memoised: opening a folder re-renders the tree, not every playlist row in it. */
const PlaylistRow = memo(function PlaylistRow({
  item,
  depth,
}: {
  item: TreePlaylist;
  depth: number;
}) {
  const p = item.playlist;
  const over = usePlaylistDrop(`playlist:${p.id}`, p);
  return (
    <li className="group/row relative flex" data-drop={`playlist:${p.id}`}>
      <Link
        to="/playlist/$id"
        params={{ id: p.id }}
        className={cn(ROW, ACTIVE, "pr-8", over && DROP_OVER)}
        style={{ paddingLeft: 10 + depth * 14 }}
      >
        <Artwork src={p.art.small} size={20} alt="" />
        <span className="truncate">{p.name}</span>
      </Link>
      <span className="absolute top-1 right-1">
        <ActionMenu kind="playlist" target={p} name={p.name} />
      </span>
    </li>
  );
});

interface FolderState {
  open: ReadonlySet<string>;
  toggle: (id: string) => void;
}

function FolderRow({
  folder,
  children,
  hidden,
  depth,
  folders,
}: {
  folder: PlaylistFolder;
  children: TreeNode[];
  hidden: number;
  depth: number;
  folders: FolderState;
}) {
  const { t } = useLingui();
  const open = folders.open.has(folder.id);
  const Icon = open ? FolderOpen : FolderClosed;
  // A drag that rests on a closed folder opens it, so its playlists can take the drop.
  const hold = useRef(0);
  const springOpen = () => {
    if (open || !useDrag.getState().active) return;
    hold.current = window.setTimeout(() => {
      if (useDrag.getState().active) folders.toggle(folder.id);
    }, 600);
  };
  const folderName = folder.name;
  return (
    <li>
      <div className="group/row relative flex">
        <button
          type="button"
          aria-expanded={open}
          onClick={() => folders.toggle(folder.id)}
          onPointerEnter={springOpen}
          onPointerLeave={() => clearTimeout(hold.current)}
          className={cn(ROW, "pr-8 text-left")}
          style={{ paddingLeft: 10 + depth * 14 }}
        >
          <span className="grid size-5 shrink-0 place-items-center">
            <Icon aria-hidden className="size-4" strokeWidth={1.75} />
          </span>
          <span className="truncate">{folder.name}</span>
          <ChevronRight
            aria-hidden
            className={cn(
              "ml-auto size-3.5 shrink-0 text-ink-faint transition-transform duration-100",
              open && "rotate-90",
            )}
            strokeWidth={1.75}
          />
        </button>
        <DropdownMenu>
          <MoreTrigger label={t`More for the folder ${folderName}`} />
          <DropdownMenuContent align="start" className="w-48">
            <DropdownMenuItem
              onSelect={() =>
                showDialog({ kind: "create", items: [], label: "", folderId: folder.id })
              }
            >
              <Trans>New playlist here</Trans>
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => showDialog({ kind: "folder", folder })}>
              <Trans>Rename…</Trans>
            </DropdownMenuItem>
            <DropdownMenuItem
              disabled={children.length + hidden > 0}
              onSelect={() => void deleteFolder(folder.id, folder.name)}
            >
              {children.length + hidden > 0 ? t`Delete (move its playlists out first)` : t`Delete…`}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      {open &&
        (children.length ? (
          <Tree nodes={children} depth={depth + 1} folders={folders} />
        ) : (
          <p className="py-1.5 text-meta text-ink-faint" style={{ paddingLeft: 36 + depth * 14 }}>
            {hidden ? (
              <Plural
                value={hidden}
                one="Its playlist is listed in another section"
                other="Its # playlists are listed in other sections"
              />
            ) : (
              <Trans>Empty folder</Trans>
            )}
          </p>
        ))}
    </li>
  );
}

function Tree({
  nodes,
  depth,
  folders,
}: {
  nodes: TreeNode[];
  depth: number;
  folders: FolderState;
}) {
  return (
    <ul className="flex flex-col gap-0.5">
      {nodes.map((n) =>
        n.kind === "folder" ? (
          <FolderRow
            key={n.folder.id}
            folder={n.folder}
            hidden={n.hidden}
            depth={depth}
            folders={folders}
          >
            {n.children}
          </FolderRow>
        ) : (
          <PlaylistRow key={n.item.playlist.id} item={n.item} depth={depth} />
        ),
      )}
    </ul>
  );
}

/** While songs, albums or playlists are dragged: a row that makes a new playlist of them. */
function NewPlaylistDrop() {
  const offered = useDragActive(addable);
  const over = useNewPlaylistDrop();
  if (!offered) return null;
  return (
    <div
      aria-hidden
      data-drop="playlist:new"
      className={cn(DROP_ZONE, "mb-0.5", over && "border-transparent bg-ink text-ground")}
    >
      <ListPlus className="size-4 shrink-0" strokeWidth={1.75} />
      <Trans>New playlist</Trans>
    </div>
  );
}

function Playlists({ lists, pending }: { lists: SidebarPlaylists; pending: boolean }) {
  const { t } = useLingui();
  const tree = usePlaylistTree();
  const folders = useOpenFolders();
  const nodes = lists.own;
  return (
    <section>
      <Heading
        action={
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={t`New playlist or folder`}
                className="-mr-1.5 text-ink-muted hover:text-ink"
              >
                <Plus />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-44">
              <DropdownMenuItem
                onSelect={() => showDialog({ kind: "create", items: [], label: "", folderId: "" })}
              >
                <Trans>New playlist…</Trans>
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => showDialog({ kind: "folder" })}>
                <Trans>New folder…</Trans>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        }
      >
        {t`Playlists`}
      </Heading>
      <NewPlaylistDrop />
      {nodes.length ? (
        <Tree nodes={nodes} depth={0} folders={folders} />
      ) : (
        !pending && (
          <p className="px-2.5 text-meta text-ink-faint">
            {tree.data?.playlists.some((p) => p.playlist.canEdit) ? (
              <Trans>All your playlists are pinned above.</Trans>
            ) : (
              <Trans>Your playlists appear here.</Trans>
            )}
          </p>
        )
      )}
    </section>
  );
}

const COLLAPSED_KEY = "sona.sidebar.collapsed";

function readCollapsed(): ReadonlySet<string> {
  try {
    return new Set(JSON.parse(localStorage.getItem(COLLAPSED_KEY) ?? "[]") as string[]);
  } catch {
    return new Set();
  }
}

/** The sections folded shut, kept across restarts; every section starts open. */
function useCollapsed(id: string): { open: boolean; toggle: () => void } {
  const [collapsed, setCollapsed] = useState(readCollapsed);
  const toggle = () =>
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (!next.delete(id)) next.add(id);
      localStorage.setItem(COLLAPSED_KEY, JSON.stringify([...next]));
      return next;
    });
  return { open: !collapsed.has(id), toggle };
}

/**
 * A flat list of library playlists the user cannot edit: no rename, no
 * folders, no drops, no "New playlist". The heading folds it shut.
 */
function ReadOnlySection({
  id,
  title,
  items,
}: {
  id: string;
  title: string;
  items: TreePlaylist[];
}) {
  const { open, toggle } = useCollapsed(id);
  const listId = `sidebar-${id}`;
  if (!items.length) return null;
  return (
    <section>
      <h2 className="mt-6 mb-1.5 h-5 text-meta text-ink-faint">
        <button
          type="button"
          aria-expanded={open}
          aria-controls={listId}
          onClick={toggle}
          className="flex h-5 w-full items-center justify-between rounded-md px-2.5 text-left transition-colors duration-100 hover:text-ink"
        >
          {title}
          <ChevronRight
            aria-hidden
            className={cn(
              "size-3.5 shrink-0 transition-transform duration-100",
              open && "rotate-90",
            )}
            strokeWidth={1.75}
          />
        </button>
      </h2>
      <ul id={listId} hidden={!open} className="flex flex-col gap-0.5">
        {open && items.map((item) => <PlaylistRow key={item.playlist.id} item={item} depth={0} />)}
      </ul>
    </section>
  );
}

/**
 * The sidebar's playlists, under the library links: Pinned, Made for You
 * (Apple's personal mixes), Apple Music (playlists added from Apple Music
 * or another curator, which cannot be edited) and the user's own Playlists
 * with their folders (a SidebarSection slot). A pinned playlist shows under
 * Pinned only.
 */
export function PlaylistSidebar() {
  const { t } = useLingui();
  const tree = usePlaylistTree();
  const pins = usePins();
  // Waiting for the pins keeps pinned playlists from showing in their
  // section for a moment first.
  const pending = tree.isPending || pins.isPending;
  const data = pins.isPending ? undefined : tree.data;
  const lists = useMemo(
    () =>
      data
        ? sidebarPlaylists(data, new Set(pins.data?.map((p) => p.id)))
        : { madeForYou: [], apple: [], own: [] },
    [data, pins.data],
  );
  return (
    <>
      <Pins />
      <ReadOnlySection id="made-for-you" title={t`Made for you`} items={lists.madeForYou} />
      <ReadOnlySection id="apple-music" title={t`Apple Music`} items={lists.apple} />
      <Playlists lists={lists} pending={pending} />
    </>
  );
}
