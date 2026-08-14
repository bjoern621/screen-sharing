# bruno

The HTTP APIs of a running deployment, as a [Bruno](https://usebruno.com) collection.

Two of them, in a folder each.
`Group service` is this repository's own API, served by `backend/cmd/groupd` and described in `backend/internal/groupsvc`.
`Relay` is the MediaMTX API the app polls for who is live, read by `backend/internal/relay`, plus the HLS playlist as the one media leg a GET can check.

The control contract between the backend and the shell is not here and is not HTTP.
It is gRPC over a Unix socket, stated in `docs/ipc-api.md` and defined in `api/`.

## Running it

Open the directory as a collection in the Bruno app, or run it headless:

```bash
bru run --env Local
```

Pick an environment first.
`Local` points at a relay and a group service on loopback, on the ports `mediamtx.yml` and `groupd`'s defaults use.
`Production` points at the deployed relay behind the reverse proxy, where one name on the standard port fronts the group service and the HLS listener alike.
That name is the proxy's `SCREENSHARE_DOMAIN`, so another deployment is this environment with its own domain in both fields.

`relayApi` stays on loopback in both.
The proxy does not front the relay's API and the relay binds it to loopback, so the `Relay` folder wants a tunnel and a credential of its own against a deployment:

```bash
task relay:tunnel
sh scripts/relay-api-token.sh <relay host> 2h
```

The task forwards the API port off the deployment `Production` points at, and `task relay:tunnel RELAY_HOST=<relay host>` names another one.
Opening a tunnel that is already open is a no-op, and `task relay:tunnel:stop` closes it.

A stale tunnel keeps listening while it forwards nothing, so a request that hangs instead of refusing is one to restart it for.

The printed token goes in `relayApiToken`, and it grants the API alone.
A group token grants publishing and reading under one prefix and never the API, which is why the operator's is signed separately and on the machine the signing key is on.

## Order

`Create group` and then `Issue token`, before anything else.
Each writes what the later requests read: the group key, the prefix its streams live under, and the relay token the media legs carry.

The key and the token are declared secret, so Bruno keeps their values out of the environment files and this collection carries no credential.
The rest of an environment is addresses and a stream name, which is why the files are committed.
