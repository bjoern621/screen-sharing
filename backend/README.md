# backend

The Go module: the headless backend the shell talks to, and the group service that runs beside the relay.

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
  internal/        what the two draw on
  go.mod           module bjoernblessin.de/screenshare
```

The module path does not carry the directory name: it is `bjoernblessin.de/screenshare`, so no import line names `backend`.
`api` is reached from here by a filesystem `replace` on `../api`, which is why the Nix builds read the module cache rather than a vendor directory (`nix/package.nix`).

## Two binaries, one module

The two land on different machines and ship as different packages (`nix/package.nix`, `nix/groupd.nix`).
That separation is the `cmd/` directories, not a module boundary.

They stay one module because what they share is a contract rather than convenience code.
`internal/group` derives a stream's path prefix from the group key, and both sides run that derivation: the backend for the prefix it publishes under, the service for the prefix it grants a token on.
Two implementations of one hash issue a member a token for a path nobody publishes to.
`internal/token` is the same story for the token the relay verifies.

Splitting the two into their own modules would move those packages out of `internal/`, since `internal/` is enforced at the module boundary.
A shared derivation would become published surface of the repository, which is the opposite of what a split is for.
The contract that genuinely has two independent consumers is the wire schema, and that already has a module of its own in `api/`.
