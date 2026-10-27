// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"strconv"
	"strings"
	"time"
)

// Meta is an object's metadata block on the wire: the store's own
// fields, under the Kubernetes names, beside the product's flat JSON.
// An object's ResourceVersion is its revision as a decimal string.
//
// ResourceVersion is optional in a schema because a write body need not
// carry it. A write names its object in the path and the revision it
// read in If-Match, so the block would repeat both. The server sends it
// on every object it answers.
type Meta struct {
	Name              string    `json:"name"`
	Namespace         string    `json:"namespace,omitempty"`
	ResourceVersion   string    `json:"resourceVersion" required:"false"`
	CreationTimestamp time.Time `json:"creationTimestamp,omitzero"`
	// Writer wrote the object's revision; Writer.At is the last write's
	// time, so there is no separate update time.
	Writer *Writer `json:"writer,omitempty"`
	// Set is the object's membership, absent on a backend without sets.
	Set *MetaSet `json:"set,omitempty"`
}

// MetaSet is a membership on the wire, with the mode as its string.
type MetaSet struct {
	Name string `json:"name"`
	Mode string `json:"mode"`
}

// ListMeta is a list's metadata block. ResourceVersion is the cursor of
// the snapshot the items were read at ([FormatCursor]).
type ListMeta struct {
	ResourceVersion string `json:"resourceVersion"`
}

// FormatCursor is a list's or a bookmark's resourceVersion:
// "<revision>.<epoch>". The epoch rides inside so a client cannot resume
// from a revision of another history, and cannot drop half the token.
func FormatCursor(rev uint64, epoch string) string {
	return strconv.FormatUint(rev, 10) + "." + epoch
}

// ParseCursor splits a cursor at its first ".". "" and "0" are a watch
// from the current state. Anything else needs a revision and an epoch:
// an object's resourceVersion alone is refused with an [InvalidError],
// because one set write gives several objects one revision and resuming
// after it could skip their siblings.
func ParseCursor(s string) (rev uint64, epoch string, err error) {
	if s == "" || s == "0" {
		return 0, "", nil
	}
	revText, epoch, ok := strings.Cut(s, ".")
	rev, perr := strconv.ParseUint(revText, 10, 64)
	if !ok || perr != nil || epoch == "" {
		return 0, "", &InvalidError{Reason: "resourceVersion " + strconv.Quote(s) + " is not a cursor; resume from a list or bookmark resourceVersion"}
	}
	return rev, epoch, nil
}

// Item pairs a decoded value with its metadata, for a seam whose
// implementation cannot import the product's wire type.
type Item[T any] struct {
	Meta  Meta
	Value T
}

// Projected is one watch event of decoded values: Value is set on Added
// and Modified, Meta alone on Deleted, Cursor alone on Bookmark. A
// server projects its store's events into it, and a client decodes a
// watch stream's lines into it.
type Projected[T any] struct {
	Type   EventType
	Meta   Meta
	Value  T
	Cursor string
}
