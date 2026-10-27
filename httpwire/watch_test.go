// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package httpwire

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeWatch(t *testing.T) {
	stream := `{"type":"ADDED","object":{"metadata":{"name":"a","resourceVersion":"4"},"value":"v"}}
{"type":"BOOKMARK","object":{"metadata":{"resourceVersion":"4.e"}}}
`
	var got []Event
	for ev, err := range DecodeWatch(strings.NewReader(stream)) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, ev)
	}
	if len(got) != 2 || got[0].Type != EventAdded || got[1].Type != EventBookmark {
		t.Fatalf("%+v", got)
	}
	if raw, ok := got[1].Object.(json.RawMessage); !ok || string(raw) != `{"metadata":{"resourceVersion":"4.e"}}` {
		t.Fatalf("bookmark object %s", got[1].Object)
	}

	var errs int
	for _, err := range DecodeWatch(strings.NewReader("{\"type\":\"ADDED\"}\nnot json\n{}\n")) {
		if err != nil {
			errs++
		}
	}
	if errs != 1 {
		t.Fatalf("%d errors, want the stream to end at the bad line", errs)
	}
}
