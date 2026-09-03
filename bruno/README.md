# bruno

HTTP APIs of a running deployment, as a [Bruno](https://usebruno.com) collection.

Two of them, in three folders.
`Group service` is this repository's own API, served by `backend/cmd/groupd`, described in `backend/internal/groupsvc`.
`Members` is the same service's membership half: who is in a group, and closing what anybody holding no lease has open (`backend/internal/membership`).
`Relay` is the MediaMTX API the app polls for who is live, read by `backend/internal/relay`, plus the HLS playlist as the one media leg a GET can check.

The backend-to-shell contract is elsewhere: gRPC over a Unix socket, stated in `docs/ipc-api.md`, defined in `api/`.

## Running it

Open the directory as a collection in the Bruno app, or headless:

```bash
bru run --env Local
```

Pick an environment first.

| Environment | Points at |
| --- | --- |
| `Local` | relay and group service on loopback, on the ports `deploy/mediamtx-groups.yml` and `groupd`'s defaults use |
| `Production` | the deployed relay behind the reverse proxy, one name on the standard port fronting the group service and the HLS listener alike |

That name is the proxy's `SCREENSHARE_DOMAIN`, so another deployment is this environment with its own domain in both fields.

## What each environment reaches

`Local` runs every folder.
`Production` runs `Group service` alone: those routes are paths under the deployment's name (`deploy/Caddyfile`).

The relay's API and `/members` answer on 9997 and 9443 behind that name, closed to everything off the host, so `Relay` and `Members` are Local's.
Sent to the public name they reach the HLS listener, which answers `{"status":"error","error":"authentication error"}`.
That shape is MediaMTX's, so a refusal carrying it is a request that never arrived here.

## The relay API credential

The relay grants its API to nothing a group token carries (`deploy/mediamtx-groups.yml`), so the `Relay` folder holds an operator's own token.
`task relay` draws the signing key, and `groupd` prints a token off it:

```bash
bin/groupd -api-token 2h -key dev-relay/signing-key.pem
```

The value goes in `relayApiToken` without the scheme word, the folder header carrying that.
A relay access token is the other grant, covering publishing and reading under one prefix.

## Order

`Create group`, then `Issue token`, before anything else.
Each writes what the later requests read: the group key, the prefix its streams live under, the member secret this collection knows itself by, and the relay access token the media legs carry.

The `Members` folder runs in its own order on top of that, and wants a stream open to be worth watching.
`Claim a name`, `Refresh presence`, then `A name another member holds` for the 409, `View the members`, and `Leave the group`.
Run `Leave the group` while a second member holds a lease and the leaver's connections come back under `kicked` and their player stops.

A member secret is drawn by the request that needs one where the environment holds none, which is what an app does on its first join to a group.
The secret is what makes a member unforgeable inside the group, so it is drawn locally and the service never sees one.

Group key, member secrets and tokens are declared secret, so Bruno keeps their values out of the environment files and this collection carries no credential.
The rest of an environment is addresses, a stream name and a display name, so the files are committed.
