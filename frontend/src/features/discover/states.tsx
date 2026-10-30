import { ErrorBlock } from "@/shared/components/ErrorBlock";

/** Flat placeholder bars at the real sizes of a page of shelves. */
export function ShelvesSkeleton({ hero = false }: { hero?: boolean }) {
  return (
    <div aria-busy aria-label="Loading" className="flex flex-col gap-10">
      {hero && <div className="aspect-[4.8/1] w-full rounded-sm bg-hover" />}
      {Array.from({ length: 3 }, (_, s) => (
        <div key={s}>
          <div className="mb-3 h-4 w-40 rounded-sm bg-hover" />
          <div className="grid [grid-auto-rows:0] grid-cols-[repeat(auto-fill,minmax(160px,1fr))] [grid-template-rows:auto] gap-x-6 overflow-hidden">
            {Array.from({ length: 10 }, (_, i) => (
              <div key={i}>
                <div className="aspect-square rounded-sm bg-hover" />
                <div className="mt-2 h-3.5 w-3/4 rounded-sm bg-hover" />
                <div className="mt-1.5 h-3 w-1/2 rounded-sm bg-hover" />
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

/** The page-level failure: what failed, Go's reason, Retry. */
export function PageError({
  what,
  error,
  onRetry,
  retrying,
}: {
  what: string;
  error: unknown;
  onRetry: () => void;
  retrying: boolean;
}) {
  return (
    <ErrorBlock
      title={`Couldn't load ${what}`}
      error={error}
      onRetry={onRetry}
      retrying={retrying}
    />
  );
}
