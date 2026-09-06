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
                    │   /groups /tokens /streams /members   │───MoQ────────► browser
                    │   /jwks.json ──► groupd, loopback     │   HTTP/3, not
                    │   */whip */whep ──► MediaMTX WebRTC   │   through Caddy
                    │   /webrtc/* ──► its page, prefix cut  │
                    │   everything else ──► MediaMTX HLS    │
                    │                                       │
                    │ MediaMTX API, loopback, operator only │
                    └───────────────────────────────────────┘
```

## What crosses the internet

Exposed are the legs no reverse proxy can carry, and the one port that is the proxy.
Every one is encrypted by something of its own, since none is behind the certificate on 443.

RTSP and RTMP are not HTTP, so each terminates TLS in the relay itself, on a listener of its own.
The certificate is the proxy's, handed to the relay by the deployment rather than issued a second time.
An encrypted RTSP session carries its RTP interleaved in that connection.
RTSPS wraps the control channel alone, so media over UDP would travel beside it in the clear.
TCP is the encrypted session's only lower transport rather than its slower one.

SRT is UDP with no TLS at all, so what protects it is a passphrase rather than a certificate, one per path prefix.
A group's passphrase derives from its group key on both ends, so nobody sets or sees one.
The app keys its legs with it, and the group service writes the same derivation into the relay's path configuration.
A publish holding no readable group key derives nothing, and an encrypted relay refuses that leg rather than sending it in the clear.

WebRTC media negotiates a direct UDP path to the viewer, which is the point of it, so it never meets the proxy either.
It is DTLS-SRTP by construction.

Its signalling and its player page do go through the proxy, both being ordinary HTTP.
The page answers under `/webrtc/`, the prefix being what tells it from the HLS page: the two are the same path on the same name once 443 has replaced the listener's port.
The proxy strips it again, so the relay serves the path it knows and nothing there is aware of the prefix.

Media over QUIC is HTTP and still not proxied.
Its session is a CONNECT over HTTP/3, which a proxy listening on TCP 443 never sees, so the relay answers 8892 itself.
TCP for the player page, UDP for the WebTransport session, one number for both.
TLS is not optional there either, WebTransport refusing a plaintext listener.

Everything else is HTTP and answers on 443 under one certificate.

## Two certificates on the MoQ listener

A deployment configures one of the two.

The page over TCP is ordinary TLS, validated against a CA like any other site.
It takes the proxy's certificate, handed over the way RTSPS and RTMPS take it, and shows an interstitial without one.
`moqServerCert` and `moqServerKey` name it, and a host points them at the pair it already has through `MTX_MOQSERVERCERT` and `MTX_MOQSERVERKEY`.
The override is what keeps the path out of a config file every deployment reads.

The session over UDP is pinned instead: the page reads the listener's SHA-256 off `/fingerprint` and passes it in `serverCertificateHashes`.
That is how a browser accepts a certificate no CA vouches for.
Pinning bounds what that certificate may be, since nothing can revoke one: no RSA key, and no validity longer than 14 days.
MediaMTX therefore generates it rather than taking it from configuration, ECDSA P-256 rotated inside the window, and there is nothing here to renew.

## What answers on loopback alone

The relay's API, the group service, and the scrape.
The cleartext RTSP and RTMP listeners are not bound at all: the relay sets `strict` on both, so there is nothing on those ports to reach rather than something a firewall is hiding.
The API is not a member's endpoint: a group token grants publishing and reading under one prefix and names no API action, so an exposed API would refuse every caller it could reach.
Reading it takes an operator's own token, on the machine the relay runs on (`tools/bruno/README.md`).

The scrape names every group's prefix, the streams under it and how many read each, and it asks for no credential.
So the address it binds is the whole of what gates it, and a deployment whose collector sits on another host widens it and says in the same place who may reach it (`deploy/mediamtx-groups.yml`).

The port numbers live in `deploy/mediamtx-groups.yml` and the routing in `deploy/Caddyfile`.
Those two files are what every relay obeys, a deployment and a development machine alike.

## Checking a relay

A check dials every leg the settings address and reports what each one answered.
The app draws it under the connection step's controls, on the settings on screen.
`backend check-relay` runs the same check over the stored ones and prints them.

```
–  discord                             Discord mode is off
✓  groups   https://relay.example      200 OK           0.9.0  62ms
✓  hls      https://relay.example      200 OK                  64ms
✓  webrtc   https://relay.example      204 No Content          64ms
✓  rtsp     rtsps://relay.example:8322 RTSP/1.0 200 OK         60ms
✗  srt      srt://relay.example:8890   i/o timeout             5.001s
```

Each row is about the listener the transport carrying it names, so a check reaches what a stream reaches.
A port the relay does not bind is a cross here rather than a publish that waits out its connect window.

Each is asked in its own protocol, an open socket proving nothing on its own.
RTSP answers `OPTIONS`, SRT answers the induction handshake that precedes any stream id or passphrase, an HTTP leg answers a request with any status at all, and RTMPS is the TLS handshake and the certificate behind it.

An HTTP leg is dialled with the request its own server serves, so what comes back is 2xx.
One name fronts HLS, WebRTC and the two services, and a path no route claims reaches whichever server handles the rest, so a bare listener would have one leg reporting another's health.
WebRTC is asked with the `OPTIONS` a WHEP endpoint answers its ICE servers to, and HLS and MoQ with a `GET` of the player page, each under a path inside this machine's group that names no stream.
The relay serves a page before it looks for a stream, so a check needs neither a publisher nor a viewer.

The request carries the relay token, traded for the group key the way a publish trades for one.
Nothing is read without one, so a leg dialled without a token answers 401 whatever the listener behind it is doing.
A trade that fails leaves the legs dialled bare, and the group service has a row of its own to fail in.

The Discord manager is a leg where Discord mode is on, presence and tokens coming from it rather than from the group service (`discord-mode.md`).
It is asked on the one route of its own that takes no credential, every other one either refusing a check or starting a consent flow.

The version is what a listener named itself with in its `Server` header.
The group service and the manager carry the number their release was built with, and MediaMTX names itself without one.

A dash is a leg this deployment addresses nowhere, and is no failure: settings naming no relay leave every leg with one, nothing having been asked of any listener.
The relay's own API is no leg here at all, being an operator's endpoint that no publish and no viewer dials.
The command's exit status is 1 where a leg that was dialled did not answer.

A relay that answers on nothing still comes back as a response, every leg carrying its own verdict, which is what `CheckRelay` returns (`docs/ipc-api.md`, "Errors").

## Who holds what

A group key is what lets somebody join a group, and its digest is the path prefix every stream of that group lives under.
The group service trades that key for a relay token granting publish and read on that prefix.
The relay verifies it locally against a published key set, so nothing is called per connection.

A group key is drawn at the service on request, so a group is made by asking for one rather than by an operator writing it down anywhere.
`POST /groups` answers a fresh group key and the app reaches it through `CreateGroup`, rate limited per address.
The relay holds no list of groups, and the service stores nothing beyond the presence leases each member's own app states, the prefix being the group key's own digest.
`auth-flow.md` draws the exchanges and `membership.md` the leases.

A publisher holding no group key is refused a token, so nothing it builds reaches the relay.
The app says so before the stream starts: the commit is greyed and the reason names the group key.

Where the token rides is the protocol's answer rather than a choice:

| Protocol | Token rides in |
| --- | --- |
| RTSP, RTMP | a query |
| SRT | the stream id |
| HLS, WebRTC | an `Authorization` header |
| MoQ | an `Authorization` header for the page, a MOQT setup parameter for the session |

The HTTP legs read no query at all, so an address is not a form any of them takes.
A player page is the exception: it is handed to a browser, which sets no header of its own.
The token goes in the address as the Basic password under an arbitrary user, `https://jwt:<token>@relay:8892/<path>/`.
The MoQ page then carries it onto the subscription, that session being QUIC and holding no header to put it in.
The GStreamer RTSP reader is the other exception, taking it as the session's password instead, because it addresses each track at the SDP's control attribute and loses a query on the way.
Each transport states its own credential form, and `plan.md` covers the group model in full.

Every named relay carries a group service, and its address follows the relay's: the proxy's own name off a trusted network, `http://<host>:9443` where this network reaches the relay directly (`settings.Relay.GroupService`).
A machine that has named no relay has nothing to ask, so it holds no token, derives no prefix and builds bare names.
