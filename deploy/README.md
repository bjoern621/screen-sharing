# deploy

What a relay runs on: one MediaMTX configuration, the proxy in front of it, and the hook it calls.

Every relay this repository runs reads these files,
a deployment on the internet and a development machine alike.
`docs/install.md`, "The relay", is the page for standing one up.

## Files

| File | Read by |
| --- | --- |
| `mediamtx-groups.yml` | MediaMTX, on every start. Ports, listeners, and the key set it checks each token against |
| `Caddyfile` | the reverse proxy, fronting the group service and the HLS listener under one name on 443 |
| `reconcile-on-read.sh` | MediaMTX itself, as `pathDefaults.runOnRead`, reporting each starting read to `POST /reconcile` |
| `relay.sh`, `relay.ps1` | a person starting a relay on this machine |

A deployment mounts none of this.
The container images carry the files (`packaging/nix/relay-image.nix`, `packaging/nix/proxy-image.nix`).

## Running one here

```sh
task relay
```

The relay and the group service come up together, and Ctrl+C ends both.
Neither serves anything alone:
the relay verifies every connection against the key set the service publishes,
so a relay started by itself refuses every publisher.

A self-signed certificate and a signing key are drawn into `dev-relay/` where none is there, and kept.
A certificate trusted once stays trusted,
and a token issued before a restart still verifies after one.
The certificate and the hook path reach MediaMTX as environment overrides,
so the file itself stays the one a deployment reads.

## Reports

The app's crash reporter delivers bundles to `POST /reports`,
which groupd stores as files in the directory its `-reports` flag names.
The flag left empty refuses the route,
so a deployment that wants them mounts a volume and names it there.
The development scripts store them in `dev-relay/reports`.

Windows takes `pwsh deploy/relay.ps1`, which fetches `mediamtx.exe` into `bin/` on first run.
Elsewhere both binaries come from the flake's dev shell.

## The rest

`docs/network-architecture.md` for which leg is encrypted with what,
`docs/auth-flow.md` for what a token grants,
`docs/membership.md` for the lease the reconcile hook enforces.
`tools/bruno` calls these APIs against a running relay.
