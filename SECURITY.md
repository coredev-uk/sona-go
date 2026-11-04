# Security policy

## Reporting a vulnerability

Report vulnerabilities privately through GitHub:
[Report a vulnerability](https://github.com/coredev-uk/sona-go/security/advisories/new)
(the repository's Security tab → Advisories → Report a vulnerability). Do
not open a public issue, pull request or discussion for a vulnerability.

Include what is affected (version from `sona --version` or Settings →
Diagnostics, platform, install method), the steps to reproduce, and what an
attacker gains. Leave out your Apple Music token, cookies and any account
data; the diagnostics bundle redacts them, but check before attaching it.

You will get an answer in the advisory. Once a fix is released, the advisory
is published with credit to you unless you ask otherwise.

## Supported versions

Sona is before 1.0. Only the latest release gets security fixes; a fix ships
in a new release, not as a patch to older ones. Development builds from
`main` are fixed on `main`.

## Scope

In scope: Sona's own code and its release packages. For example, leaking the
Apple Music token or other credentials (to logs, files, the UI or the
network), the loopback sign-in server, the CDM download and its verification
(Mozilla's manifest, SHA-512, CRX3 signatures), the CDM host process and its
sandbox, `music://` link and `sona` command handling, settings import, and
the update check. [docs/security-review.md](docs/security-review.md) lists
the known findings and the threat model.

Out of scope: vulnerabilities in Apple's services, in Google's Widevine CDM,
or in MyGo, GTK3, GStreamer or other dependencies (report those
upstream; tell us if Sona needs to update or work around them), and attacks
that need an already compromised user account.

### DRM

Sona plays protected music only through Google's Widevine CDM, downloaded
from Google and used as is. Report problems in how Sona hosts it: the
sandbox escaping, decrypted audio leaving the CDM host process or reaching
disk, the downloader accepting an unverified file. Never ask for, send or
publish Widevine keys, device certificates, key-extraction techniques or
ways around Apple's or Google's protection. Such reports are closed, and
contributions doing that are not accepted.
