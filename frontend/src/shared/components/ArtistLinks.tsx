import type { Artist } from "@bindings/services";
import { Link } from "@tanstack/react-router";
import { Fragment } from "react";

/**
 * Every credited artist as a link to their page: "A, B & C". Without
 * artists it shows `fallback` (Apple's combined artist name) as text.
 */
export function ArtistLinks({
  artists,
  fallback,
}: {
  artists: readonly Artist[];
  fallback: string;
}) {
  if (!artists.length) return <>{fallback}</>;
  return (
    <>
      {artists.map((a, i) => (
        <Fragment key={a.id}>
          {i > 0 && (i === artists.length - 1 ? " & " : ", ")}
          <Link
            to="/artist/$id"
            params={{ id: a.id }}
            className="hover:underline hover:underline-offset-2"
          >
            {a.name}
          </Link>
        </Fragment>
      ))}
    </>
  );
}
