import type { Settings } from "@bindings/config";
import { useLingui } from "@lingui/react/macro";
import { ChevronDown, ChevronUp } from "lucide-react";
import { useRef } from "react";

import { Button } from "@/shared/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Switch } from "@/shared/ui/switch";

import { moveProvider } from "./lyrics-order";
import { useSettings, useUpdateSettings } from "./queries";
import { SELECT_WIDTH, SettingRow, SettingsGroup } from "./SettingRow";

/** Languages offered for translations ("" is the Apple Music account's). */
const translationLanguages = [
  "ar",
  "de",
  "en",
  "es",
  "fr",
  "he",
  "hi",
  "id",
  "it",
  "ja",
  "ko",
  "nl",
  "pl",
  "pt",
  "pt-BR",
  "ru",
  "sv",
  "th",
  "tr",
  "uk",
  "vi",
  "zh-Hans",
  "zh-Hant",
];

/** The language's name in the interface's language; the tag when Intl can't name it. */
function languageName(tag: string, locale: string): string {
  try {
    return new Intl.DisplayNames([locale], { type: "language" }).of(tag) ?? tag;
  } catch {
    return tag;
  }
}

/**
 * Settings → Playback → Lyrics: which sources Sona asks for lyrics, in
 * which order, whether word-by-word timing wins over the order, and the
 * language translations come in.
 */
export function LyricsRows() {
  const { t, i18n } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  const buttons = useRef(new Map<string, HTMLButtonElement>());
  const s = settings.data;
  if (!s?.lyrics) return null;
  const lyrics = s.lyrics;
  const save = (patch: Partial<Settings["lyrics"]>) =>
    update.mutate({ ...s, lyrics: { ...lyrics, ...patch } });

  const names: Record<string, { name: string; about: string }> = {
    apple: {
      name: t`Apple Music`,
      about: t`Word-by-word and line-by-line lyrics, with translations and pronunciation.`,
    },
    musixmatch: {
      name: t`Musixmatch`,
      about: t`Word-by-word and line-by-line lyrics, with translations from its community.`,
    },
    lrclib: { name: t`LRCLIB`, about: t`Line-by-line lyrics from an open, shared database.` },
    netease: {
      name: t`NetEase Cloud Music`,
      about: t`Line-by-line lyrics, strongest for Chinese, Japanese and Korean music.`,
    },
  };

  const move = (i: number, dir: -1 | 1) => {
    const id = lyrics.providers[i].id;
    const to = i + dir;
    save({ providers: moveProvider(lyrics.providers, i, dir) });
    // Keep focus on the button that moved the row, or on its other button
    // once the row reaches an end.
    const atEnd = to === 0 || to === lyrics.providers.length - 1;
    const key = `${id}:${atEnd ? -dir : dir}`;
    requestAnimationFrame(() => buttons.current.get(key)?.focus());
  };
  const keep = (key: string) => (el: HTMLButtonElement | null) => {
    if (el) buttons.current.set(key, el);
    else buttons.current.delete(key);
  };

  const collator = new Intl.Collator(i18n.locale);
  const languages = [...translationLanguages];
  if (lyrics.translationLanguage && !languages.includes(lyrics.translationLanguage)) {
    languages.push(lyrics.translationLanguage);
  }
  const named = languages
    .map((tag) => ({ tag, name: languageName(tag, i18n.locale) }))
    .sort((a, b) => collator.compare(a.name, b.name));
  const ACCOUNT = "account";

  return (
    <SettingsGroup
      id="playback-lyrics"
      title={t`Lyrics`}
      description={t`Sona asks every source that's on and shows the lyrics with the finest timing: word by word, then line by line, then plain text. A source higher in the list wins a tie.`}
    >
      <ul aria-label={t`Lyrics sources`} className="divide-y divide-line">
        {lyrics.providers.map((p, i) => {
          const about = names[p.id] ?? { name: p.id, about: "" };
          const name = about.name;
          const rowId = `lyrics-${p.id}`;
          return (
            <li key={p.id} className="flex items-center justify-between gap-6 py-4">
              <div className="min-w-0">
                <label htmlFor={rowId} className="text-ui font-medium text-ink">
                  {name}
                </label>
                <p id={`${rowId}-desc`} className="mt-0.5 max-w-[56ch] text-sub text-ink-muted">
                  {about.about}
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <Button
                  ref={keep(`${p.id}:-1`)}
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t`Move ${name} up`}
                  disabled={i === 0}
                  onClick={() => move(i, -1)}
                >
                  <ChevronUp strokeWidth={1.75} />
                </Button>
                <Button
                  ref={keep(`${p.id}:1`)}
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t`Move ${name} down`}
                  disabled={i === lyrics.providers.length - 1}
                  onClick={() => move(i, 1)}
                >
                  <ChevronDown strokeWidth={1.75} />
                </Button>
                <Switch
                  id={rowId}
                  className="ms-3"
                  aria-describedby={`${rowId}-desc`}
                  checked={p.enabled}
                  onCheckedChange={(enabled) =>
                    save({
                      providers: lyrics.providers.map((q) =>
                        q.id === p.id ? { ...q, enabled } : q,
                      ),
                    })
                  }
                />
              </div>
            </li>
          );
        })}
      </ul>
      <SettingRow
        id="lyrics-word-timing"
        title={t`Prefer word-by-word lyrics`}
        description={t`Lyrics timed word by word win over line-by-line lyrics from a source higher in the list.`}
      >
        <Switch
          id="lyrics-word-timing"
          aria-describedby="lyrics-word-timing-desc"
          checked={lyrics.preferWordTiming}
          onCheckedChange={(preferWordTiming) => save({ preferWordTiming })}
        />
      </SettingRow>
      <SettingRow
        id="lyrics-translation"
        title={t`Lyrics translation`}
        description={t`Shown under each line where a source has a translation into this language.`}
      >
        <Select
          value={lyrics.translationLanguage || ACCOUNT}
          onValueChange={(v) => save({ translationLanguage: v === ACCOUNT ? "" : v })}
        >
          <SelectTrigger
            id="lyrics-translation"
            aria-describedby="lyrics-translation-desc"
            className={SELECT_WIDTH}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ACCOUNT}>{t`Your Apple Music language`}</SelectItem>
            {named.map((l) => (
              <SelectItem key={l.tag} value={l.tag}>
                {l.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </SettingRow>
    </SettingsGroup>
  );
}
