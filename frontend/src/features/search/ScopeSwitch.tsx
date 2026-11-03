import { useLingui } from "@lingui/react/macro";

import { Segmented } from "@/shared/components/Segmented";

import type { SearchScope } from "./search-params";

/** Where the search looks: the Apple Music catalogue or the library on this computer. */
export function ScopeSwitch({
  scope,
  onChange,
}: {
  scope: SearchScope;
  onChange: (scope: SearchScope) => void;
}) {
  const { t } = useLingui();
  return (
    <Segmented
      value={scope}
      label={t`Search in`}
      onChange={onChange}
      options={[
        { value: "catalog", label: t`Apple Music` },
        { value: "library", label: t`Library` },
      ]}
    />
  );
}
