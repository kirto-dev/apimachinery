# kirto.dev/apimachinery

The client side of the Kirto resource model: the wire types, typed
errors and HTTP contract that a Kirto server's resource routes serve. It
holds no server, no store and no product vocabulary, and it imports
nothing outside the standard library, so a client, an SDK or an
operator can depend on it alone.

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

The split follows Kubernetes' apimachinery: types and errors in one
package that both sides import, and the transport contract beside it.

## License

Apache-2.0. See [LICENSE](LICENSE).
