# api

The control contract between the Go backend and every shell in front of it.

This directory is the schema and nothing else: no business logic, no transport code, no shell code.
It is a separate module so both sides depend on the contract rather than on each other.

`docs/ipc-api.md` states the rule the contract encodes - a shell shows what the backend describes and asks the backend to act, and decides nothing - along with the transport, the error model and the versioning policy.
Read that first. What follows is how to work in this directory.

## Layout

```
api/
  proto/screenshare/v1/     the schema, one file per concern
    settings.proto            StreamSettings and Preset
    catalog.proto             the fixed facts: codecs, decoders, monitors, carriage
    form.proto                Form: every field, option, greying and reason, decided
    session.proto             the running state: publish, relay, viewers
    events.proto              Event, the server-push envelope
    control.proto             ControlService, the whole callable surface
  gen/go/screenshare/v1/    generated Go, committed
  buf.yaml                  lint and breaking-change configuration
  buf.gen.yaml              what buf generates
  go.mod                    module bjoernblessin.de/screenshare/api
```

The `.proto` files carry the reasoning. A message comment says why the shape is what it is, not what the fields are named - the names say that themselves.

## Generating

```bash
task api
```

That runs `buf lint`, `buf breaking` against `main`, and `buf generate`.
The Go output is committed, so `go build` works on a fresh clone with no protobuf toolchain installed; CI checks the committed output is current.

The C# side is not committed and not generated here. `Grpc.Tools` compiles these same `.proto` files during the Avalonia build:

```xml
<ItemGroup>
  <PackageReference Include="Grpc.Net.Client" Version="..." />
  <PackageReference Include="Google.Protobuf" Version="..." />
  <PackageReference Include="Grpc.Tools" Version="..." PrivateAssets="all" />
  <Protobuf Include="../../api/proto/screenshare/v1/*.proto"
            ProtoRoot="../../api/proto"
            GrpcServices="Client" />
</ItemGroup>
```

One schema, two generators, no hand-written copy on either side.

Tooling comes from the flake's dev shell on Linux and macOS. On Windows, `buf`, `protoc-gen-go` and `protoc-gen-go-grpc` are installed separately; `protoc` itself is not needed, since `buf` carries its own compiler.

The C# side is the exception, and only on Nix. Grpc.Tools runs prebuilt `protoc` and `grpc_csharp_plugin` binaries out of its own NuGet package, and those are linked against an interpreter NixOS does not have, so the dev shell points it at the packaged pair through `Protobuf_ProtocFullPath` and `gRPC_PluginFullPath`. On Windows the bundled binaries run as shipped and nothing is overridden.

## Changing the contract

Within `v1`, changes are additive. New fields take new numbers, no number is reused, and no field changes type or meaning.
`buf breaking` enforces it, so a change that would break a shipped shell fails before it is merged rather than after it is deployed.

A change that cannot be additive is a `v2`: a new directory beside this one, both served for as long as a shell needs the old one.

Adding a settings field is three edits and they belong together:

1. the field in `StreamSettings`, with the comment saying what it means and why it is its own field;
2. the `Field` the backend emits for it from `ResolveForm`, with its label, help text and options;
3. the backend rule that reads it.

A field added to the message and not to the form is a value no shell can set. A field added to the form and not to the message is a control with nowhere to write.
