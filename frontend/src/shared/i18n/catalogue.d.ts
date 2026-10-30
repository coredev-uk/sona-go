/** Message catalogues, compiled to JS by @lingui/vite-plugin on import. */
declare module "*.po" {
  export const messages: import("@lingui/core").Messages;
}

interface ImportMetaEnv {
  /** Forces a locale with a catalogue, e.g. "en-XA" for the pseudo-locale. */
  readonly VITE_SONA_LOCALE?: string;
}
