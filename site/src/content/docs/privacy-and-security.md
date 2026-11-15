---
title: Privacy and Security
description: What Sona stores, where, and how to report a vulnerability.
---

Sona runs on your computer and talks to Apple, and to the services you connect. It has no server of its own.

## What Sona stores

| What | Where |
| --- | --- |
| Settings, your themes | The config folder |
| Library copy, history and caches | The data and cache folders |
| Logs, the saved queue | The state folder |
| Apple Music sign-in, Last.fm and ListenBrainz sign-ins | Your system keyring |

Sona makes every folder readable by you only. [Troubleshooting](/guide/troubleshooting/) lists where they are. Sona never stores your Apple Account password: it uses it for one exchange with Apple.

## Your sign-in

Sona keeps your Apple Music token in the system keyring: the Secret Service on Linux (GNOME Keyring or KWallet), the Keychain on macOS, the Credential Manager on Windows. If the keyring is not working, Sona stores the token in a file only you can read and warns you that it is unencrypted. Sona moves it into the keyring when one is available. Signing out deletes the token.

## Private listening

[Private listening](/guide/private-listening/) keeps plays out of your Apple history, scrobbles and Discord.

## Widevine

Apple Music's songs are protected with Widevine, Google's software. Sona downloads it from Google on first start, checks its signature, and runs it in a separate, sandboxed helper process that has no network access and cannot write to your files. Sona does not bundle, modify or extract anything from it.

## Report a vulnerability

Report it privately: [Report a vulnerability](https://github.com/coredev-uk/sona-go/security/advisories/new). Do not open a public issue. The [security policy](https://github.com/coredev-uk/sona-go/blob/main/SECURITY.md) says what to include, and [docs/security-review.md](https://github.com/coredev-uk/sona-go/blob/main/docs/security-review.md) holds the threat model.
