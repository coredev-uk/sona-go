import { Trans, useLingui } from "@lingui/react/macro";

import { type LanguageOption, offersLanguages } from "@/shared/i18n";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";

import { useLanguageOptions, useSetLanguage } from "./language-queries";
import { useSettings } from "./queries";
import { SELECT_WIDTH, SettingRow, SettingsPanel } from "./SettingRow";

/** The select's value for Automatic (Radix Select needs a non-empty value). */
const AUTOMATIC = "automatic";

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
  return (
    <Select value={value || AUTOMATIC} onValueChange={(v) => onChange(v === AUTOMATIC ? "" : v)}>
      <SelectTrigger id={id} aria-describedby={`${id}-desc`} className={SELECT_WIDTH}>
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
 * Sona's language: Settings → General's first row, and the first-run
 * setup's welcome step. Offered once a language besides English is
 * complete enough to choose (or while one is chosen). Go's own text (tray,
 * notifications, the command line, file dialogs) and Apple Music's content
 * follow it.
 */
export function LanguagePanel({ className }: { className?: string }) {
  const { t } = useLingui();
  const current = useSettings().data?.language ?? "";
  const options = useLanguageOptions(current);
  const setLanguage = useSetLanguage();
  if (!options || !offersLanguages(options, current)) return null;
  return (
    <SettingsPanel className={className}>
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
