// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package httpwire

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net"
)

// ContentTypeWatch is a watch stream's content type: one JSON [Event]
// per line.
const ContentTypeWatch = "application/json;stream=watch"

// The event types on the wire.
const (
	EventAdded    = "ADDED"
	EventModified = "MODIFIED"
	EventDeleted  = "DELETED"
	EventBookmark = "BOOKMARK"
	EventError    = "ERROR"
)

// Event is one line of a watch stream. Object is the route's wire object
// on ADDED and MODIFIED, {"metadata": Meta} on DELETED (name, namespace
// and resourceVersion only), {"metadata": {"resourceVersion": cursor}}
// on BOOKMARK, and a [Status] on ERROR, after which the stream ends.
type Event struct {
	Type   string `json:"type"`
	Object any    `json:"object"`
}

// MaxWatchLine bounds one line [DecodeWatch] reads.
const MaxWatchLine = 8 << 20

// DecodeWatch reads a watch stream's lines. Each event's Object is the
// line's object as a json.RawMessage. The sequence ends at the end of
// the body, and with an error at a line that does not decode. A read
// that ended because the reader's context was cancelled or its body
// closed ends the sequence without one.
func DecodeWatch(rd io.Reader) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		sc := bufio.NewScanner(rd)
		sc.Buffer(nil, MaxWatchLine)
		for sc.Scan() {
			var l struct {
				Type   string          `json:"type"`
				Object json.RawMessage `json:"object"`
			}
			if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
				yield(Event{}, fmt.Errorf("watch line %q: %w", sc.Text(), err))
				return
			}
			if !yield(Event{Type: l.Type, Object: l.Object}, nil) {
				return
			}
		}
		if err := sc.Err(); err != nil && !isClosed(err) {
			yield(Event{}, err)
		}
	}
}

// isClosed is a read that ended because the reader closed the stream.
func isClosed(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed)
}
