# backend

Go module holding three binaries: the headless backend the shell talks to, the group service beside the relay, and the Discord manager beside that.

Every Go command runs from this directory.

```bash
go build ./...
go vet ./...
```

## Layout

```
backend/
  cmd/backend/     the process the shell starts: capture, encode, publish and decode
  cmd/groupd/      the key, token and index service, installed on the relay's machine
  cmd/discordd/    the Discord manager: voice channels as groups (docs/discord-mode.md)
  internal/        what the three draw on
  go.mod           module bjoernblessin.de/screenshare
```

Module path drops the directory name: `bjoernblessin.de/screenshare`, so no import line names `backend`.
`api` is reached by a filesystem `replace` on `../api`, so the Nix builds read the module cache rather than a vendor directory (`packaging/nix/package.nix`).

## Separate binaries, one module

The binaries land on different machines and ship as different packages (`packaging/nix/package.nix`, `packaging/nix/groupd.nix`).
That separation is the `cmd/` directories.
`discordd` stands beside `groupd` and speaks to it over its public routes alone,
so the two share the wire and nothing else.

One module, because what they share is a contract rather than convenience code.
`internal/group` derives a stream's path prefix from the group key, and both sides run that derivation.
The backend takes the prefix it publishes under, the service the prefix it grants a token on.
Two implementations of one hash issue a member a token for a path nobody publishes to.
`internal/token` is the same for the token the relay verifies.

Splitting them into separate modules moves those packages out of `internal/`, which is enforced at the module boundary.
A shared derivation would become published surface of the repository, the opposite of what a split is for.
The contract with two independent consumers is the wire schema, and that has its own module in `api/`.
