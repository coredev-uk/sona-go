import { Trans, useLingui } from "@lingui/react/macro";

import { SonaMark } from "@/shared/brand/SonaMark";
import { Button } from "@/shared/ui/button";

import { Panel, StepActions, StepHeader } from "./parts";

/** The first step: what Sona is, and the three things the setup does. */
export function WelcomeStep({ onNext }: { onNext: () => void }) {
  const { t } = useLingui();
  const items = [
    {
      term: t`Apple Music`,
      what: t`Sign in for your library, your playlists and whole songs.`,
    },
    {
      term: t`Widevine`,
      what: t`Google's component that plays Apple Music's protected songs. Sona downloads it by itself.`,
    },
    {
      term: t`Listening`,
      what: t`Private listening, Sound Check and crossfade.`,
    },
  ];
  return (
    <>
      <SonaMark className="mb-5 size-10" />
      <StepHeader title={t`Welcome to Sona`}>
        <Trans>
          Sona plays Apple Music on this computer. Setting it up takes about a minute; every step
          can be skipped, and changed later in Settings.
        </Trans>
      </StepHeader>
      <Panel>
        <dl className="divide-y divide-line">
          {items.map((i) => (
            <div key={i.term} className="grid grid-cols-[8rem_minmax(0,1fr)] gap-6 py-3.5">
              <dt className="text-ui font-medium text-ink">{i.term}</dt>
              <dd className="text-ui text-ink-muted">{i.what}</dd>
            </div>
          ))}
        </dl>
      </Panel>
      <StepActions>
        <Button size="sm" onClick={onNext}>
          <Trans>Get started</Trans>
        </Button>
      </StepActions>
    </>
  );
}
