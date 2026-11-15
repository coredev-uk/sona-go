---
title: Architecture Overview
description: How Sona is put together, and the rules every change keeps.
---

Sona is a desktop Apple Music client written in Go, for Linux, macOS and Windows. [MyGo](https://github.com/egoist/mygo) draws the interface natively on the GPU, in the same process that holds the services, the library and the playback coordinator. The full design, with the reasons, is [docs/architecture.md](https://github.com/coredev-uk/sona-go/blob/main/docs/architecture.md).

## The three parts

| Part | Where | Does |
| --- | --- | --- |
| UI | `internal/ui` | Draws the interface and calls the services directly. Holds no credentials and never calls `*.apple.com`. |
| Go core | `internal/services` and the packages under `internal/` | Owns all Apple Music traffic, sign-in and tokens, the SQLite cache, library sync, playback coordination, MPRIS, artwork and desktop integration. |
| CDM host | `internal/playback/cdmhost` | A child process, the same executable started with `SONA_CDMHOST=1`. Loads the Widevine CDM, decrypts and plays protected audio. Makes no network requests. |

The parent fetches everything and passes bytes to the child. Decrypted audio never leaves the child; only decoded music video pictures leave it, in memory, for display.

## Where code lives

| Path | Holds |
| --- | --- |
| `cmd/sona` | The app. `main.go` is portable; `platform_*.go` holds what differs per OS. |
| `internal/services` | Services, DTOs and events. The contract is in `doc.go`. |
| `internal/applemusic` | The only Apple Music client. |
| `internal/db` | SQLite, goose migrations and sqlc queries (`dbq` is generated). |
| `internal/playback` | The engine contract, the coordinator and the CDM host. |
| `internal/ui` | The MyGo UI: `app` (shell), `module`, `store`, `theme`, `kit`, `i18n` and `features/<name>`. |
| `docs/` | Contributor and agent references. |

`VERSION` is the version source.

## Rules every change keeps

- Go owns all Apple HTTP traffic and secrets. Secrets go to the system keyring (or the 0600 file fallback), never to the database, logs or committed files.
- Protected playback runs only in the CDM host. Sona never bundles, redistributes or patches the CDM, and never extracts keys.
- Cold start reads SQLite only. The network and the playback host start after the first paint.
- MyGo draws on the GPU: no layout, text shaping or image work per frame. Decode artwork at its shown size, off the main thread. Pre-blurred backdrops come from Go.
- OS-specific code sits behind the platform seams (`platform_*.go`, `services.Platform`). Where a platform lacks a feature, report it with `errors.ErrUnsupported` and hide or explain the control. Never write a stub that pretends.
- Go's MPRIS name is the only Sona player on D-Bus.

## Go further

- [docs/architecture.md](https://github.com/coredev-uk/sona-go/blob/main/docs/architecture.md): process model, playback and DRM, data and sync, performance, platforms.
- [docs/auth.md](https://github.com/coredev-uk/sona-go/blob/main/docs/auth.md): sign-in.
- [docs/security-review.md](https://github.com/coredev-uk/sona-go/blob/main/docs/security-review.md): the threat model.
- [docs/platforms.md](https://github.com/coredev-uk/sona-go/blob/main/docs/platforms.md): build tags and per-OS setup.
