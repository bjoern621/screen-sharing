# bruno

HTTP APIs of a running deployment, as a [Bruno](https://usebruno.com) collection.

Two of them, a folder each.
`Group service` is this repository's own API, served by `backend/cmd/groupd`, described in `backend/internal/groupsvc`.
`Relay` is the MediaMTX API the app polls for who is live, read by `backend/internal/relay`, plus the HLS playlist as the one media leg a GET can check.

The backend-to-shell contract is not here and is not HTTP: gRPC over a Unix socket, stated in `docs/ipc-api.md`, defined in `api/`.

## Running it

Open the directory as a collection in the Bruno app, or headless:

```bash
bru run --env Local
```

Pick an environment first.

| Environment | Points at |
| --- | --- |
| `Local` | relay and group service on loopback, on the ports `mediamtx.yml` and `groupd`'s defaults use |
| `Production` | the deployed relay behind the reverse proxy, one name on the standard port fronting the group service and the HLS listener alike |

That name is the proxy's `SCREENSHARE_DOMAIN`, so another deployment is this environment with its own domain in both fields.

`relayApi` stays on loopback in both.
The proxy does not front the relay's API and the relay binds it to loopback, so the `Relay` folder needs a tunnel and a credential of its own:

```bash
task relay:tunnel
sh scripts/relay-api-token.sh <relay host> 2h
```

The task forwards the API port off the deployment `Production` names, and `task relay:tunnel RELAY_HOST=<relay host>` names another.
Opening an open tunnel is a no-op. `task relay:tunnel:stop` closes it.
A stale tunnel keeps listening while forwarding nothing, so a request that hangs rather than refuses is one to restart it for.

The printed token goes in `relayApiToken` and grants the API alone.
A group token grants publishing and reading under one prefix and never the API, so the operator's is signed separately, on the machine holding the signing key.

## Order

`Create group`, then `Issue token`, before anything else.
Each writes what the later requests read: the group key, the prefix its streams live under, and the relay token the media legs carry.

Key and token are declared secret, so Bruno keeps their values out of the environment files and this collection carries no credential.
The rest of an environment is addresses and a stream name, which is why the files are committed.
