import { useLingui } from "@lingui/react/macro";
import { TranslationsService } from "@bindings/services";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import {
  appleShareNeeded,
  communityShares,
  type LanguageOption,
  languageOptions,
} from "@/shared/i18n";
import { keys } from "@/shared/lib/keys";

import { useSettings, useUpdateSettings } from "./queries";

/**
 * The languages the Language setting offers for the chosen one, once Go
 * and the community catalogues have answered (undefined until then): Go
 * lists Apple's locales only when they could be complete enough, so most
 * of the time the answer is English alone and the setting stays hidden.
 */
export function useLanguageOptions(current: string): LanguageOption[] | undefined {
  const { i18n } = useLingui();
  const need = appleShareNeeded();
  const apple = useQuery({
    queryKey: [...keys.translationsLanguages, need],
    queryFn: () => TranslationsService.Languages(need),
  });
  const community = useQuery({ queryKey: keys.translationsCommunity, queryFn: communityShares });
  if (apple.isPending || community.isPending) return undefined;
  return languageOptions(apple.data, community.data ?? [], current, i18n.locale);
}

/**
 * Saves the Language setting; Apple's content follows it too (l=), so every
 * query refetches in the new language.
 */
export function useSetLanguage() {
  const qc = useQueryClient();
  const settings = useSettings();
  const update = useUpdateSettings();
  return (language: string) => {
    const s = settings.data;
    if (!s || s.language === language) return;
    update.mutate({ ...s, language }, { onSuccess: () => void qc.invalidateQueries() });
  };
}
