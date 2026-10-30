import {
  PlaylistsService,
  type Pin,
  type PlaylistFolder,
  type TreePlaylist,
} from "@bindings/services";
import { Link, useNavigate } from "@tanstack/react-router";
import { ChevronRight, Ellipsis, FolderClosed, FolderOpen, Plus } from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";

import { ActionMenu } from "@/shared/actions/ActionMenu";
import { Artwork } from "@/shared/components/Artwork";
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
import { usePins, usePlaylistTree } from "./queries";
import { buildTree, type TreeNode } from "./tree";

const row =
  "flex h-8 min-w-0 flex-1 items-center gap-2.5 rounded-md px-2.5 text-ui text-ink-muted transition-colors duration-100 hover:bg-hover hover:text-ink";
const active =
  "data-[status=active]:bg-active data-[status=active]:font-medium data-[status=active]:text-ink";

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

const pinActions = [
  { value: "play", label: "Play" },
  { value: "shuffle", label: "Shuffle" },
  { value: "drillIn", label: "Open" },
] as const;

/** `upAfter`: the pin to place this one after to move it up ("" the top; null when first). */
function PinRow({ pin, upAfter }: { pin: Pin; upAfter: string | null }) {
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
    <li className="group/row relative flex">
      {opens && link ? (
        <Link {...link} className={cn(row, active, "pr-8")}>
          {art}
          <span className="truncate">{pin.name}</span>
        </Link>
      ) : (
        <button type="button" onClick={activate} className={cn(row, "pr-8 text-left")}>
          {art}
          <span className="truncate">{pin.name}</span>
          <span className="sr-only">{pin.action === "shuffle" ? ", shuffles" : ", plays"}</span>
        </button>
      )}
      <DropdownMenu>
        <MoreTrigger label={`More for ${pin.name}`} />
        <DropdownMenuContent align="start" className="w-48">
          <DropdownMenuLabel className="text-meta font-normal text-ink-faint">
            When chosen
          </DropdownMenuLabel>
          <DropdownMenuRadioGroup
            value={pin.action}
            onValueChange={(v) =>
              void run("Couldn't change the pin", PlaylistsService.SetPinAction(pin.id, v))
            }
          >
            {pinActions.map((a) => (
              <DropdownMenuRadioItem key={a.value} value={a.value}>
                {a.label}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
          <DropdownMenuSeparator />
          {link && !opens && (
            <DropdownMenuItem onSelect={() => void navigate(link)}>
              {pin.kind === "song" ? "Go to album" : "Open"}
            </DropdownMenuItem>
          )}
          {upAfter !== null && (
            <DropdownMenuItem
              onSelect={() =>
                void run("Couldn't move the pin", PlaylistsService.MovePin(pin.id, upAfter))
              }
            >
              Move up
            </DropdownMenuItem>
          )}
          <DropdownMenuItem onSelect={() => void unpin(pin.id, pin.name)}>Unpin</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </li>
  );
}

function Pins() {
  const pins = usePins();
  const list = pins.data ?? [];
  if (!list.length) return null;
  return (
    <section aria-label="Pinned">
      <Heading>Pinned</Heading>
      <ul className="flex flex-col gap-0.5">
        {list.map((p, i) => (
          // Moving up one place puts the pin after the one two above it.
          <PinRow key={p.id} pin={p} upAfter={i === 0 ? null : i === 1 ? "" : list[i - 2].id} />
        ))}
      </ul>
    </section>
  );
}

const OPEN_KEY = "sona.playlist-folders.open";

function useOpenFolders(): FolderState {
  const [open, setOpen] = useState<ReadonlySet<string>>(() => {
    try {
      return new Set(JSON.parse(localStorage.getItem(OPEN_KEY) ?? "[]") as string[]);
    } catch {
      return new Set();
    }
  });
  const toggle = (id: string) =>
    setOpen((prev) => {
      const next = new Set(prev);
      if (!next.delete(id)) next.add(id);
      localStorage.setItem(OPEN_KEY, JSON.stringify([...next]));
      return next;
    });
  return { open, toggle };
}

function PlaylistRow({ item, depth }: { item: TreePlaylist; depth: number }) {
  const p = item.playlist;
  return (
    <li className="group/row relative flex">
      <Link
        to="/playlist/$id"
        params={{ id: p.id }}
        className={cn(row, active, "pr-8")}
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
}

interface FolderState {
  open: ReadonlySet<string>;
  toggle: (id: string) => void;
}

function FolderRow({
  folder,
  children,
  depth,
  folders,
}: {
  folder: PlaylistFolder;
  children: TreeNode[];
  depth: number;
  folders: FolderState;
}) {
  const open = folders.open.has(folder.id);
  const Icon = open ? FolderOpen : FolderClosed;
  return (
    <li>
      <div className="group/row relative flex">
        <button
          type="button"
          aria-expanded={open}
          onClick={() => folders.toggle(folder.id)}
          className={cn(row, "pr-8 text-left")}
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
          <MoreTrigger label={`More for the folder ${folder.name}`} />
          <DropdownMenuContent align="start" className="w-48">
            <DropdownMenuItem
              onSelect={() =>
                showDialog({ kind: "create", items: [], label: "", folderId: folder.id })
              }
            >
              New playlist here
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => showDialog({ kind: "folder", folder })}>
              Rename…
            </DropdownMenuItem>
            <DropdownMenuItem
              disabled={children.length > 0}
              onSelect={() => void deleteFolder(folder.id, folder.name)}
            >
              {children.length > 0 ? "Delete (move its playlists out first)" : "Delete…"}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      {open &&
        (children.length ? (
          <Tree nodes={children} depth={depth + 1} folders={folders} />
        ) : (
          <p className="py-1.5 text-meta text-ink-faint" style={{ paddingLeft: 36 + depth * 14 }}>
            Empty folder
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
          <FolderRow key={n.folder.id} folder={n.folder} depth={depth} folders={folders}>
            {n.children}
          </FolderRow>
        ) : (
          <PlaylistRow key={n.item.playlist.id} item={n.item} depth={depth} />
        ),
      )}
    </ul>
  );
}

function Playlists() {
  const tree = usePlaylistTree();
  const nodes = useMemo(() => (tree.data ? buildTree(tree.data) : []), [tree.data]);
  const folders = useOpenFolders();
  return (
    <section aria-label="Playlists">
      <Heading
        action={
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label="New playlist or folder"
                className="-mr-1.5 text-ink-muted hover:text-ink"
              >
                <Plus />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-44">
              <DropdownMenuItem
                onSelect={() => showDialog({ kind: "create", items: [], label: "", folderId: "" })}
              >
                New playlist…
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => showDialog({ kind: "folder" })}>
                New folder…
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        }
      >
        Playlists
      </Heading>
      {nodes.length ? (
        <Tree nodes={nodes} depth={0} folders={folders} />
      ) : (
        !tree.isPending && (
          <p className="px-2.5 text-meta text-ink-faint">Your playlists appear here.</p>
        )
      )}
    </section>
  );
}

/** The sidebar's pins and playlist tree (a SidebarSection slot). */
export function PlaylistSidebar() {
  return (
    <>
      <Pins />
      <Playlists />
    </>
  );
}
