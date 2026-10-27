// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

// Package resource is the client side of the Kirto resource model: the
// types a resource route's objects, lists and watch events carry on the
// wire, the typed errors a refusal decodes to, and the name rules a
// server applies. It is the shape of the Kubernetes apiserver's wire
// without the apiserver: typed objects addressed by kind, namespace and
// name, each written under compare-and-swap on its own revision, read
// with List and followed with Watch from a cursor.
//
// The package names no kind and serves nothing. The store, its backends
// and the routes that serve them are the server's; this package is what
// the server and its clients agree on, so a client's errors.Is matches
// the refusal the server meant.
//
// # Revisions
//
// One monotonic store revision stamps every write, across every kind and
// namespace. An object's revision is the store revision of the last
// write that touched it, so a name deleted and created again never
// repeats a revision a stale writer could hold. Revisions strictly
// increase and may have gaps, and no store head is 0, the
// [Precondition.StoreRevision] that means no fence.
//
// # Writes
//
// Every write is a compare-and-swap. [Precondition.Revision] 0 is
// create; a non-zero one must equal the stored revision. A delete always
// names a revision.
//
// # Wire
//
// An object is the product's own flat JSON with a metadata block,
// [Meta], whose resourceVersion is its revision as a decimal string. A
// list's and a bookmark's resourceVersion is a cursor,
// "<revision>.<epoch>" ([FormatCursor]), so a watch resumes in the
// history it was read from or is refused as too old; a watch resumes
// only from a cursor, because one set write gives several objects one
// revision.
//
// Errors on the wire are [Status], this package's own form with a
// string code; it is not the Kubernetes Status object. Package httpwire
// holds the HTTP half: the status table, preconditions, ETags and the
// watch stream's lines.
package resource
