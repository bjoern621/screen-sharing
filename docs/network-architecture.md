# Network architecture

Streams never travel between machines directly.
Every publisher sends one copy to a relay, and every viewer reads from that relay.

## Why a relay

**Reachability.** Both ends sit behind NAT, and a home connection behind CGNAT cannot accept an inbound connection at all.
A relay is the one address both ends can reach, so neither has to be reachable itself.
Connecting peers directly instead means hole punching, a signalling channel to do it through, and a relay to fall back to when it fails, which is a relay plus the machinery to sometimes avoid it.

**Uplink.** A publisher's upstream carries one copy of the stream whatever the number of viewers, because the fan-out happens at the relay.
Sending to viewers directly multiplies the publisher's upstream by their count, and a desktop uplink is the scarcest link in the path.

**One protocol in, many out.** The relay terminates what it is given and re-muxes for each listener, so a stream published over SRT is watchable over HLS or WebRTC by a browser that cannot speak SRT.
Which formats survive which leg is per transport, and `docs/domain-model.md` carries that table.

**One place that decides.** A token is checked at the relay's handshake, so membership is enforced once rather than per peer pair.
The relay is also the only thing that knows which streams exist, which is what the app's stream list and the group index both read.

The cost is that nothing is end to end.
The relay decrypts, re-muxes and re-encrypts, so it sees plaintext by construction, and the group service that signs its tokens can mint one for any group.
`docs/plan.md` states that trade-off where the group model is described.

## The shape

```
  publisher                        relay host                         viewers
                    ┌───────────────────────────────────────┐
   app ──SRT/UDP───►│ MediaMTX                              │───SRT/UDP────► app
   token in the     │   terminates every protocol and       │───RTSP───────► player
   stream id,       │   re-muxes per listener               │
   passphrase on    │                                       │
   the wire         │ Caddy, TLS and ACME for every         │───HLS────────► browser
                    │ HTTP leg on 443                       │
                    │   /groups /tokens /streams /jwks.json │───WHEP───────► browser
                    │        ──► groupd, loopback           │   plus direct
                    │   */whip */whep ──► MediaMTX WebRTC   │   UDP media
                    │   everything else ──► MediaMTX HLS    │
                    │                                       │
                    │ MediaMTX API, loopback, operator only │
                    └───────────────────────────────────────┘
```

## What crosses the internet, and what does not

Exposed are the two legs that cannot go through a reverse proxy and the one port that is the proxy.
SRT is UDP with no TLS, so what protects it is a relay-wide passphrase rather than a certificate.
WebRTC media negotiates a direct UDP path to the viewer, which is the point of it, so it never meets the proxy either.
Everything else is HTTP and answers on 443 under one certificate.

Loopback-only are the relay's API, the group service, and the cleartext RTSP and RTMP listeners.
The API is not a member's endpoint: a group token grants publishing and reading under one prefix and names no API action, so an exposed API would refuse every caller it could reach.
Reading it takes an operator's own token and a tunnel, which `bruno/README.md` covers.

The port numbers live in `mediamtx.yml` for a LAN relay and `deploy/mediamtx-groups.yml` for a relay on the internet, and the routing in `deploy/Caddyfile`.
Those files are what a deployment obeys; this page is the reason they are shaped that way.

## Who holds what

A group key is membership, and its digest is the path prefix every stream of that group lives under.
The group service trades that key for a relay token granting publish and read on that prefix, and the relay verifies it locally against a published key set, so nothing is called per connection.

Where the token rides is the protocol's answer rather than a choice: a query for RTSP and RTMP, the stream id for SRT, and an `Authorization` header for HLS and WebRTC.
`backend/internal/transport/credential.go` states each, and `docs/plan.md` covers the group model in full.

A relay with no proxy in front of it has no group service, no token and no prefix.
That is the LAN shape, where anybody who can reach the machine may publish, and it is a different deployment rather than a weaker setting on this one.
