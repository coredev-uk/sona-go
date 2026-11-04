# Sona

An Apple Music client for the desktop, built first for Linux: a native
Go app whose interface is drawn by [MyGo](https://github.com/egoist/mygo).

Sona is in early development, not a finished product. There are no releases
yet. You need an Apple Music subscription. Sona is not affiliated with Apple
or Google.

| Platform             | Status                                     |
| -------------------- | ------------------------------------------ |
| Linux, x86_64        | Plays; media controls, notifications         |
| macOS, Apple Silicon | Development builds from source; plays      |
| Windows              | The UI builds; no playback yet             |

## Build and run

You need [Nix](https://nixos.org/download) and [devenv](https://devenv.sh)
2.4 or newer; devenv provides the rest of the toolchain.

```sh
git clone https://github.com/coredev-uk/sona-go.git && cd sona-go
devenv shell -- build    # builds bin/sona
devenv shell -- dev      # builds the development build and runs it
```

Development builds start in private listening, so plays aren't reported to
Apple or scrobbled.

## Playback and the Widevine CDM

Apple Music's tracks are protected with Widevine. On first start Sona
downloads Google's Widevine CDM from Google's servers, verifies it, and runs
it in a sandboxed process. The CDM is Google's software: it isn't part of
this repository or any Sona build, and must never be bundled or modified.

## More

- [CONTRIBUTING.md](CONTRIBUTING.md): environment, checks and pull requests
- [docs/cli.md](docs/cli.md): `sona play`, `sona status --follow` and the
  other commands for the running Sona (scripts, Waybar, Polybar)
- [docs/architecture.md](docs/architecture.md): how Sona is put together
- [SECURITY.md](SECURITY.md): reporting a vulnerability

Licensed under the GNU Affero General Public License v3.0 only
([LICENSE](LICENSE)); third-party notices in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
