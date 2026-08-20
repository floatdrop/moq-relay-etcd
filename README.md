# moq-relay-etcd

A MOQT relay whose cross-instance discovery is backed by an
[etcd](https://etcd.io) v3 cluster, plus the `DiscoveryStore` implementation it
is built on. Several relays sharing one cluster route across each other: each
advertises its local tracks and namespaces into etcd and follows peers'
advertisements on demand.

- **`.`** — `etcd.Store`, an implementation of `moq-go`'s
  `relay.DiscoveryStore` interface: leases, watches, and the key layout.
- **[`cmd/relay-etcd`](cmd/relay-etcd)** — the binary, and its operational
  documentation.

## Why it is not in moq-go

The etcd client pulls in a large dependency tree — gRPC, bbolt, zap, protobuf —
that the core [`moq-go`](https://github.com/floatdrop/moq-go) module
deliberately excludes, so that only operators who opt into etcd-backed
discovery pay for that weight. It lived in-tree as a separate Go module for the
same reason before moving here.

`moq-go` keeps the `DiscoveryStore` interface and an in-memory backend; nothing
in it imports this repository.

## Building and testing

```sh
go build ./...
go test -race ./...
```

The tests are hermetic — they run an embedded in-process etcd server, so no
daemons are required.

## Dependency on moq-go

`moq-go` publishes no tags, so this module pins it by pseudo-version. To move to
a newer `moq-go`:

```sh
go get github.com/floatdrop/moq-go@<commit-sha>
go mod tidy
```
