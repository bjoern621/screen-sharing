# The group service, built from this repository.
#
# It is built here rather than pulled because the path derivation has to be identical on both
# sides: the app computes the prefix it publishes under and this computes the prefix it grants
# a token for, and two builds of one hash is a member holding a token for a path nobody is
# publishing to (backend/internal/group).
#
# CGO is off, unlike the backend's build. This binary links no GStreamer and no X11: it draws
# keys, signs tokens and reads the relay's HTTP API, so a static binary on a distroless base is
# the whole runtime it needs.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -C backend -o /groupd ./cmd/groupd

FROM gcr.io/distroless/static-debian12
COPY --from=build /groupd /groupd
# The signing key lives on a volume the compose file mounts here, so a restart keeps issuing
# tokens the relay's cached key still verifies.
VOLUME /state
EXPOSE 9443
ENTRYPOINT ["/groupd"]
