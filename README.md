# kirto.dev/apimachinery

The client side of the Kirto resource model: the wire types, typed
errors and HTTP contract that a Kirto server's resource routes serve, and
a typed client for them. It holds no server, no store and no product
vocabulary, and it imports nothing outside the standard library, so a
client, an SDK or an operator can depend on it alone.

The servers live in `kirto.dev/platform`, whose resource store, routes
and conformance suites are written against these types, so a client's
`errors.Is` matches the refusal the server meant and the JSON both sides
read is one definition.

## Packages

- `resource`: the wire types and errors. `Meta`, `MetaSet` and
  `ListMeta` are the metadata blocks on an object and a list;
  `FormatCursor` and `ParseCursor` are a list's or bookmark's
  `<revision>.<epoch>` cursor; `Item[T]` and `Projected[T]` are a
  decoded object and a decoded watch event. `Key`, `Precondition`,
  `Writer`, `Membership`, `SetMode`, `EventType`, `PrincipalKind` and
  `BlobRef` are what those reference. The typed errors (`ErrNotFound`,
  `*ConflictError` and the rest) convert to and from `Status`, the
  error body with a string code. `ValidName` and its siblings are the
  name rules every server applies.
- `httpwire`: the HTTP half. The status table (`HTTPStatus`,
  `GenericCode`) and the error body (`Status`, `Error`), written with
  `WriteError` and read back with `ReadError`, whose error unwraps to the
  typed error; `ParsePrecondition` and `SetPrecondition` for `If-Match`
  and `If-None-Match: *`; `ETag`; and the watch stream's lines
  (`ContentTypeWatch`, the event names, `Event`, `DecodeWatch`).
- `client`: a typed client. `client.New(baseURL, Options{...})` takes
  the base URL, an `*http.Client` and a header hook for the bearer and a
  namespace; `c.Collection[T]("things")` is a `Collection[T]` with plain
  `Get`, `List`, `Watch`, `Put`, `Create` and `Delete` methods, so a
  caller can fake it behind its own interface. `c.Do` sends a route
  that is not collection CRUD and reads its answer whole; `c.Stream`
  hands back the answer's body open, for a download or a stream, and a
  `RawReader` body is sent as it is read, for an upload. Every request
  is one `Stream` underneath. It never follows a redirect.

The split follows Kubernetes' apimachinery: types and errors in one
package that both sides import, the transport contract beside it, and
the client on top, so a server imports the first two without the third.

## Requirements

Go 1.27, for the generic method `Client.Collection`.

## License

Apache-2.0. See [LICENSE](LICENSE).
