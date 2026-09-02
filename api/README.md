# api

Control contract between the Go backend and every shell in front of it.

Schema and nothing else.
A separate module, so both sides depend on the contract rather than on each other.

`docs/ipc-api.md` states the rule the contract encodes, the transport, the error model and the versioning policy.
Read it first.
This page is how to work in this directory.

## Layout

```
api/
  proto/screenshare/v1/     the schema, one file per concern
    settings.proto            Settings, its groups and Preset
    catalog.proto             the fixed facts: codecs, decoders, monitors, carriage
    form.proto                Form: every field, option, greying and reason, decided
    session.proto             the running state: publish, relay, viewers
    events.proto              Event, the server-push envelope
    control.proto             ControlService, the whole callable surface
    frame.proto               FrameService: the frame channel's handles and loans
  gen/go/screenshare/v1/    generated Go, committed
  buf.yaml                  lint and breaking-change configuration
  buf.gen.yaml              what buf generates
  go.mod                    module bjoernblessin.de/screenshare/api
```

Reasoning lives in the `.proto` files: a message comment says why a shape is what it is.

## Generating

```bash
task api
```

Runs `buf lint`, `buf breaking` against `main`, then `buf generate`.
Go output is committed, so `go build` works on a fresh clone with no protobuf toolchain.
CI checks it is current.

C# comes out of the Avalonia build, where `Grpc.Tools` compiles the same `.proto` files:

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

## Toolchain

Tooling comes from the flake's dev shell on Linux and macOS.
On Windows, `buf`, `protoc-gen-go` and `protoc-gen-go-grpc` are installed separately.
`protoc` is not needed, `buf` carrying its own compiler.

C# on Nix is the exception.
Grpc.Tools runs prebuilt `protoc` and `grpc_csharp_plugin` binaries out of its own NuGet package, linked against an interpreter NixOS does not have.
The dev shell points it at the packaged pair through `Protobuf_ProtocFullPath` and `gRPC_PluginFullPath`.
On Windows the bundled binaries run as shipped.

## Changing the contract

Within `v1`, changes are additive.
New fields take new numbers, no number is reused, no field changes type or meaning.
`buf breaking` enforces it, so a change that would break a shipped shell fails before merge.

A change that cannot be additive is a `v2`: a new directory beside this one, both served for as long as a shell needs the old one.

Adding a settings field is three edits, and they belong together:

1. the field in the settings group it belongs to, with the comment saying what it means and why it is its own field;
2. the `Field` the backend emits for it from `ResolveForm`, with its label, help text and options;
3. the backend rule that reads it.

A field in the message and not the form is a value no shell can set.
A field in the form and not the message is a control with nowhere to write.
