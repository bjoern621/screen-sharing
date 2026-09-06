# Paths and auth

Group key is the secret.
Path is public and rides in every URL.

Path is `<group-id>/<name>`, and the group id is a digest of the group key.

## Make a group, then publish

```mermaid
sequenceDiagram
    participant A as App
    participant G as groupd
    participant M as MediaMTX

    M->>G: GET /jwks.json
    Note over M: cached, refreshed hourly

    A->>G: POST /groups
    G-->>A: groupKey, 32 bytes, and groupId
    Note over A: prefix = digest of the group key

    A->>G: POST /tokens, sends groupKey and memberSecret
    G-->>A: JWT granting prefix, subject is this member's id, 5 min
    A->>M: publish prefix/name, token attached
    M->>M: check signature, match path
    M-->>A: live
```

groupd is never called per connection.
The relay holds the key set ahead of the handshake and refreshes it on a timer, so a connection is checked by arithmetic.

## Watch

```mermaid
sequenceDiagram
    participant V as Viewer
    participant G as groupd
    participant M as MediaMTX

    V->>G: GET /streams, groupKey in query
    G->>M: API, list paths
    M-->>G: every path on the relay
    Note over G: keeps children of prefix only
    G-->>V: this group's streams

    V->>G: POST /tokens, sends groupKey and memberSecret
    G-->>V: JWT granting prefix
    V->>M: read prefix/name, token attached
    M-->>V: video
```

The viewer never reaches the relay API.
Filtering happens at groupd, so a listing carries one group.

## Outsider knows the path

```mermaid
sequenceDiagram
    participant X as Outsider
    participant M as MediaMTX
    participant G as groupd

    X->>M: GET prefix/name, no token
    M-->>X: 401

    X->>G: POST /tokens, no groupKey
    G-->>X: 400, a stream lives in a group

    Note over X: takes the group key, 256 bits
```

Read is an authenticated action on every listener, not only on the API.
Knowing a group's path buys a 401, and there is no path outside a group to know.
A browser reaches a stream on the same terms, its address carrying the token as userinfo.

## Leaving

A token cannot be taken back.
The relay reads one at the handshake and not again, so a connection outlives its token and a client that is closed opens another with the same one.

Membership is therefore enforced by closing connections, against the presence leases the same service holds.
`membership.md` covers what states a lease, what a run closes and what it leaves alone.

## What each door takes

| Door | Takes | Path alone enough |
| --- | --- | --- |
| `POST /groups` | nothing, rate limited per address | makes a fresh group, reaches no existing one |
| `POST /tokens` | group key, and the member secret naming who asks | no, the request has no path field at all |
| `GET /streams` | group key | no, a prefix is not a group key |
| `PUT /members`, `DELETE /members`, `GET /members` | group key, and the member secret on the two that state and release | no |
| `POST /reconcile` | nothing, on loopback only | a path names a group and buys a run against the leases that group's own members stated, answered with no content |
| publish | JWT | no, the grant is `~^prefix` and the relay matches it |
| read | JWT | no, the grant is `~^prefix` and the relay matches it |

## What a leak costs

One grant covers both actions, so a leak publishes into the group as well as reading it.
A leaked token lasts five minutes.
A leaked group key lasts until the group draws a new one, which is `POST /groups` again.
A leaked member secret on its own buys nothing: every route that takes one takes the group key beside it.
The relay and groupd see plaintext throughout, since nothing here is end to end.

`network-architecture.md` covers who holds what.
