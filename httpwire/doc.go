// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

// Package httpwire is the HTTP half of the resource wire contract: what
// a resource route and its clients agree on over HTTP, with no store
// behind it.
//
// Errors: every error body is a [Status], [resource.Status] plus causes,
// with one status table ([HTTPStatus]) for the store's codes and
// [GenericCode] for a handler's own refusals. [Error] carries the HTTP
// status outside the body, so a framework that reflects it as its error
// type documents and sends exactly the Status fields. A server answers
// one with [WriteError]; a client reads one back with [ReadError], whose
// error unwraps to the typed error of package resource, so errors.Is
// matches what the server meant.
//
// Writes: [ParsePrecondition] reads If-Match and If-None-Match: * into a
// [resource.Precondition], and [SetPrecondition] writes one. A write
// that can replace or remove state a caller may have read requires one;
// missing is 428, stale is 412. [SetETag] tags every response that
// carries one object.
//
// Watch: GET <collection>?watch=true[&resourceVersion=<cursor>]
// [&timeoutSeconds=S] streams one JSON [Event] per line, of content type
// [ContentTypeWatch]. Without a resourceVersion the stream is the current
// state as ADDED, a BOOKMARK, then changes; from a cursor it is a
// BOOKMARK at the cursor, then every change after it. A cursor too old
// is 410 before the stream opens, or an ERROR line once it has. On 410
// or a too_old ERROR a client lists again; on a clean end it watches
// again from its last BOOKMARK. [DecodeWatch] reads the lines.
package httpwire
