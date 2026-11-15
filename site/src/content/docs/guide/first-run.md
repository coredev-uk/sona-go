---
title: First Run and Sign-in
description: Set up Sona on first start and sign in with Apple Music.
---

The first time you start Sona, a short setup runs: Welcome, Apple Music, Playback, Connections, Appearance, Desktop, Done. You can skip any step and change it later in Settings. Settings → General opens the setup again.

## Let Sona download Widevine

Apple Music's songs are protected with Widevine, Google's software. Sona downloads it from Google on first start, checks Google's signature, runs it in a sandboxed process and keeps it up to date. The setup shows the progress. Sona needs it to play Apple Music's songs.

## Sign in with Apple Music

1. In the setup's Apple Music step, or in Settings, choose to sign in.
2. Enter your Apple Account and password. Sona uses the password for one exchange with Apple and never stores it.
3. Enter the two-factor code. Choose where it comes from: an Apple device, a text message or a phone call, depending on what your account offers.

If Sona's form can't serve your account, because it uses only security keys or Apple asks you to repair it, Sona offers sign-in in your system browser. That uses your browser's password manager, passkeys and existing Apple session. On macOS it can use your iCloud Keychain passkey with Touch ID.

If Apple refuses the sign-in, Sona says why: a wrong password, a locked account, no subscription, or a country Apple Music doesn't serve.

## Listen privately

Private listening stops Sona reporting plays to Apple and scrobbling them. Development builds start with it on.

## Next

Change Sona's colours with [Themes](/guide/themes/), or control it from a terminal with the [command line](/guide/cli/).
