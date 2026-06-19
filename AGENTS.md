# Agent Notes

## Current Repo State

- This repo is currently planning-only: `LICENSE` and `.gitignore` exist; no `README`, manifests, lockfiles, CI, or app source exist yet.
- Do not invent build/test commands until executable config exists. Re-check manifests before running commands.

## Planned Stack

- App: Rust + Tauri backend, React + TypeScript frontend, ShadCN + Tailwind UI.
- Package manager: `pnpm`; do not introduce Bun lockfiles unless the project deliberately migrates.
- Development environment: `devenv` is authoritative for Linux development; planned files are `devenv.nix`, `devenv.yaml`, and `devenv.lock`.
- Frontend state/data: TanStack Router, TanStack Query, Zustand, Zod at runtime boundaries, TanStack Virtual when long lists require it.
- Database: Turso/libSQL for normalized Apple Music media-library cache.

## Linux And Playback Constraints

- Main development target is Linux; Linux runtime feasibility comes first, not as a late packaging step.
- Stock Tauri Linux uses WebKitGTK through WRY, not a stable CEF renderer.
- `tauri-apps/cef-rs` is for a separate Linux playback spike; it is not yet a drop-in Tauri renderer.
- Do not assume Apple Music DRM playback works on Linux. Test Widevine, codecs, MusicKit JS auth, and actual playback in a minimal CEF shell before wiring playback into the app.
- Linux CEF Widevine may need a persistent `user_data_path`, `--component-updater=fast-update`, and restart after CDM download.
- Official CEF builds may lack proprietary codecs; do a licensing review before distributing a proprietary-codec CEF build.

## Design Constraints

- Follow the “Nocturnal Gallery” direction: dark editorial, asymmetric, spacious, glass/gradient layers.
- Avoid 1px borders for sectioning; use tonal surface shifts, negative space, or very subtle ghost borders only when needed for accessibility.
- Use Manrope for display/editorial text and Inter for functional UI/metadata.
- Now Playing should use the planned “playback shroud”: blurred album-art glow under a glass panel.

## Secrets And Types

- Never store Apple private keys, Music user tokens, Turso auth tokens, or DRM credentials in Turso tables or committed config.
- Prefer Rust `serde` DTOs as command type sources; generate TypeScript with `specta`/`tauri-specta` or `ts-rs` if DTO duplication becomes noisy.
- Use Zod for runtime validation at trust boundaries, not as the only backend/frontend type source.
