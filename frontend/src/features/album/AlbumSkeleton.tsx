import { useLingui } from "@lingui/react/macro";

import type { AlbumLayout } from "./layout";

/** The sleeve, head and rows at their real sizes while the album loads. */
export function AlbumSkeleton({ layout }: { layout: AlbumLayout }) {
  const { t } = useLingui();
  const cover = (
    <div
      className="shrink-0 rounded-sm bg-hover"
      style={{ width: layout.cover, height: layout.cover }}
    />
  );
  const head = (
    <div className="min-w-0 flex-1">
      <div className="h-8 w-3/5 rounded-sm bg-hover" />
      <div className="mt-3 h-4 w-1/4 rounded-sm bg-hover" />
      <div className="mt-2 h-3 w-1/3 rounded-sm bg-hover" />
      <div className="mt-6 flex gap-2">
        <div className="size-10 rounded-full bg-hover" />
        <div className="h-8 w-24 rounded-md bg-hover" />
      </div>
    </div>
  );
  const rows = Array.from({ length: 8 }, (_, i) => (
    <div key={i} className="flex h-11 items-center gap-4 px-2">
      <div className="h-3 w-5 rounded-sm bg-hover" />
      <div className="h-3.5 rounded-sm bg-hover" style={{ width: `${30 + ((i * 17) % 30)}%` }} />
    </div>
  ));
  return (
    <div aria-busy aria-label={t`Loading`}>
      {layout.wide ? (
        <div
          className="grid gap-x-10"
          style={{ gridTemplateColumns: `${layout.cover + 32}px minmax(0,1fr)` }}
        >
          <div className="p-4">{cover}</div>
          <div className="pt-4">
            {head}
            <div className="mt-6">{rows}</div>
          </div>
        </div>
      ) : (
        <>
          <div className="flex gap-5 p-5">
            {cover}
            {head}
          </div>
          <div className="mt-6">{rows}</div>
        </>
      )}
    </div>
  );
}
