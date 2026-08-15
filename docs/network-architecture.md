# Network architecture

Streams never travel between machines directly.
Every publisher sends one copy to a relay, and every viewer reads from that relay.

## Why a relay

**Reachability.** Both ends sit behind NAT, and a home connection behind CGNAT cannot accept an inbound connection at all.
A relay is the one address both ends can reach, so neither has to be reachable itself.
Connecting peers directly instead means hole punching, a signalling channel to do it through, and a relay to fall back to when it fails.
That is a relay plus the machinery to sometimes avoid it.

**Uplink.** A publisher's upstream carries one copy of the stream whatever the number of viewers, because the fan-out happens at the relay.
Sending to viewers directly multiplies that upstream by their count, and a desktop uplink is the scarcest link in the path.

**One protocol in, many out.** The relay terminates what it is given and re-muxes for each listener, so a stream published over SRT is watchable over HLS or WebRTC by a browser that cannot speak SRT.
Which formats survive which leg is per transport, and `docs/domain-model.md` carries that table.

**One place that decides.** A token is checked at the relay's handshake, so membership is enforced once rather than per peer pair.
The relay is also the only thing that knows which streams exist, which the app's stream list and the group index both read.

The cost is that nothing is end to end.
The relay decrypts, re-muxes and re-encrypts, so it sees plaintext by construction.
The group service that signs its tokens can mint one for any group.
`docs/plan.md` states that trade-off where the group model is described.

## The shape

```
  publisher                        relay host                         viewers
                    ┌───────────────────────────────────────┐
   app ──SRT/UDP───►│ MediaMTX                              │───SRT/UDP────► app
   token in the     │   terminates every protocol and       │───RTSPS──────► player
   stream id,       │   re-muxes per listener               │
   passphrase on    │   RTSPS 8322 and RTMPS 1936 carry     │───HLS────────► browser
   the wire         │   the proxy's certificate             │
                    │   MoQ 8892 carries its own            │───WHEP───────► browser
                    │                                       │   plus direct
                    │ Caddy, TLS and ACME for every         │   UDP media
                    │ HTTP leg on 443                       │
                    │   /groups /tokens /streams /jwks.json │───MoQ────────► browser
                    │        ──► groupd, loopback           │   HTTP/3, not
                    │   */whip */whep ──► MediaMTX WebRTC   │   through Caddy
                    │   everything else ──► MediaMTX HLS    │
                    │                                       │
                    │ MediaMTX API, loopback, operator only │
                    └───────────────────────────────────────┘
```

## What crosses the internet, and what does not

Exposed are the legs no reverse proxy can carry, and the one port that is the proxy.
Every one is encrypted by something of its own, since none is behind the certificate on 443.

RTSP and RTMP are not HTTP, so each terminates TLS in the relay itself, on a listener of its own.
The certificate is the proxy's, handed to the relay by the deployment rather than issued a second time.
An encrypted RTSP session carries its RTP interleaved in that connection.
RTSPS wraps the control channel alone, so media over UDP would travel beside it in the clear, and TCP is the encrypted session's only lower transport rather than its slower one.

SRT is UDP with no TLS at all, so what protects it is a relay-wide passphrase rather than a certificate.
A publish to an encrypted relay with no passphrase set is refused rather than sent.

WebRTC media negotiates a direct UDP path to the viewer, which is the point of it, so it never meets the proxy either.
It is DTLS-SRTP by construction.

Media over QUIC is HTTP and still not proxied.
Its session is a CONNECT over HTTP/3, which a proxy listening on TCP 443 never sees, so the relay answers 8892 itself.
TCP for the player page, UDP for the WebTransport session, one number for both.
TLS is not optional there either, WebTransport refusing a plaintext listener.

That listener carries two certificates, and a deployment configures one of them.

The page over TCP is ordinary TLS, validated against a CA like any other site.
It takes the proxy's certificate, handed over the way RTSPS and RTMPS take it, and shows an interstitial without one.
`moqServerCert` and `moqServerKey` name it, and a host points them at the pair it already has through `MTX_MOQSERVERCERT` and `MTX_MOQSERVERKEY`.
The override is what keeps the path out of a config file every deployment reads, the way the SRT passphrase stays out of it.

The session over UDP is pinned instead: the page reads the listener's SHA-256 off `/fingerprint` and passes it in `serverCertificateHashes`, which is how a browser accepts a certificate no CA vouches for.
Pinning bounds what that certificate may be, since nothing can revoke one: no RSA key, and no validity longer than 14 days.
MediaMTX therefore generates it rather than taking it from configuration, ECDSA P-256 rotated inside the window, and there is nothing here to renew.

Everything else is HTTP and answers on 443 under one certificate.

Loopback-only are the relay's API and the group service.
The cleartext RTSP and RTMP listeners are not bound at all: the relay sets `strict` on both, so there is nothing on those ports to reach rather than something a firewall is hiding.
Cleartext is the LAN relay's shape, the `mediamtx.yml` at the repository root.
The API is not a member's endpoint: a group token grants publishing and reading under one prefix and names no API action, so an exposed API would refuse every caller it could reach.
Reading it takes an operator's own token and a tunnel, which `bruno/README.md` covers.

The port numbers live in `mediamtx.yml` for a LAN relay and `deploy/mediamtx-groups.yml` for a relay on the internet, and the routing in `deploy/Caddyfile`.
Those files are what a deployment obeys.
This page is the reason they are shaped that way.

## Who holds what

A group key is membership, and its digest is the path prefix every stream of that group lives under.
The group service trades that key for a relay token granting publish and read on that prefix.
The relay verifies it locally against a published key set, so nothing is called per connection.

A key is drawn at the service on request, so a group is made by asking for one rather than by an operator writing it down anywhere.
`POST /groups` answers a fresh key and the app reaches it through `CreateGroup`, rate limited per address.
The service keeps no membership store and the relay holds no list of groups, since possession of the key is membership and the prefix is that key's own digest.
`auth-flow.md` draws the exchanges.

A publisher holding no key trades for a token on the public prefix instead of being refused.
That stream is authenticated at the relay and encrypted on the wire like any other.
What it lacks is a restriction on who may watch it, the one thing the app says out loud before the stream starts.

Where the token rides is the protocol's answer rather than a choice:

| Protocol | Token rides in |
| --- | --- |
| RTSP, RTMP | a query |
| SRT | the stream id |
| HLS, WebRTC, MoQ | an `Authorization` header |

The HTTP legs read no query at all, so an address is not a form any of them takes.
A player page is the exception that makes: handed to a browser, which sets no header of its own.
The token goes in the address as the Basic password under an arbitrary user, `https://jwt:<token>@relay:8892/<path>/`.
The GStreamer RTSP reader is the other exception, taking it as the session's password instead, because it addresses each track at the SDP's control attribute and loses a query on the way.
`backend/internal/transport/credential.go` states each, and `docs/plan.md` covers the group model in full.

A relay with no proxy in front of it has no group service, no token and no prefix.
That is the LAN shape, where anybody who can reach the machine may publish, and it is a different deployment rather than a weaker setting on this one.
