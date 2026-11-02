import { Trans, useLingui } from "@lingui/react/macro";
import { useEffect, useRef } from "react";

import { type LanguageOption, offersLanguages } from "@/shared/i18n";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";

import { useLanguageOptions, useSetLanguage } from "./language-queries";
import { useSettings } from "./queries";
import { SELECT_WIDTH, SettingRow, SettingsPanel } from "./SettingRow";

/** The select's value for Automatic (Radix Select needs a non-empty value). */
const AUTOMATIC = "automatic";

/**
 * A new language remounts the whole tree (`I18nRoot`), and the row only
 * comes back once its options load, so focus would fall to the page (or
 * the setup step's title). The select being changed is noted here, and its
 * new instance takes focus back when it mounts. A change that doesn't
 * remount leaves focus on the trigger anyway; the note then expires.
 */
let refocus: { id: string; until: number } | undefined;
const REFOCUS_MS = 10_000;

/** The language select: Automatic, then the languages by name in themselves. */
export function LanguageSelect({
  id,
  value,
  options,
  onChange,
}: {
  id: string;
  value: string;
  options: readonly LanguageOption[];
  onChange: (language: string) => void;
}) {
  const trigger = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (refocus?.id !== id || Date.now() > refocus.until) return;
    refocus = undefined;
    // After this commit's other effects (the setup page focuses its title).
    const frame = requestAnimationFrame(() => trigger.current?.focus());
    return () => cancelAnimationFrame(frame);
  }, [id]);
  return (
    <Select
      value={value || AUTOMATIC}
      onValueChange={(v) => {
        refocus = { id, until: Date.now() + REFOCUS_MS };
        onChange(v === AUTOMATIC ? "" : v);
      }}
    >
      <SelectTrigger ref={trigger} id={id} aria-describedby={`${id}-desc`} className={SELECT_WIDTH}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={AUTOMATIC}>
          <Trans>Automatic</Trans>
        </SelectItem>
        {options.map((o) => (
          <SelectItem key={o.tag} value={o.tag} lang={o.tag}>
            {o.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/**
 * Sona's language: Settings → General's first row (the first-run setup
 * has its own, with the same select). Offered once a language besides
 * English is complete enough to choose (or while one is chosen). Go's own
 * text (tray, notifications, the command line, file dialogs) and Apple
 * Music's content follow it.
 */
export function LanguagePanel() {
  const { t } = useLingui();
  const current = useSettings().data?.language ?? "";
  const options = useLanguageOptions(current);
  const setLanguage = useSetLanguage();
  if (!options || !offersLanguages(options, current)) return null;
  return (
    <SettingsPanel>
      <SettingRow
        id="language"
        title={t`Language`}
        description={t`Automatic follows your Apple Music account and your system. Apple Music's own text, such as album notes, follows too where your country or region offers the language.`}
      >
        <LanguageSelect id="language" value={current} options={options} onChange={setLanguage} />
      </SettingRow>
    </SettingsPanel>
  );
}
