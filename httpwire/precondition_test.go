// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package httpwire

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"kirto.dev/apimachinery/resource"
)

func TestParsePrecondition(t *testing.T) {
	for _, c := range []struct {
		ifMatch, ifNoneMatch string
		required             bool
		rev                  uint64
		status               int
	}{
		{"", "*", true, 0, 0},
		{"", " * ", false, 0, 0},
		{`"17"`, "", true, 17, 0},
		{`W/"17"`, "", true, 17, 0},
		{`"17", W/"17"`, "", true, 17, 0},
		{`"17"`, "", false, 17, 0},
		{"", "", true, 0, 428},
		{"", "", false, 0, 0},
		{`"17"`, "*", true, 0, 422},
		{"*", "", true, 0, 422},
		{`"0"`, "", true, 0, 422},
		{`"17", "18"`, "", true, 0, 422},
		{"", `"17"`, true, 0, 422},
		{"17", "", true, 0, 400},
		{`"x"`, "", true, 0, 400},
		{`""`, "", true, 0, 400},
		{`"-1"`, "", true, 0, 400},
		{`"+1"`, "", true, 0, 400},
		{`"17", junk`, "", true, 0, 400},
	} {
		pre, err := ParsePrecondition(c.ifMatch, c.ifNoneMatch, c.required)
		if c.status == 0 {
			if err != nil || pre != (resource.Precondition{Revision: c.rev}) {
				t.Errorf("If-Match %q If-None-Match %q required %v = %+v %v, want revision %d", c.ifMatch, c.ifNoneMatch, c.required, pre, err, c.rev)
			}
			continue
		}
		var e *Error
		if !errors.As(err, &e) || e.HTTPStatus != c.status || e.Code != GenericCode(c.status) {
			t.Errorf("If-Match %q If-None-Match %q required %v = %v, want %d", c.ifMatch, c.ifNoneMatch, c.required, err, c.status)
		}
	}
}

func TestPreconditionFromRequest(t *testing.T) {
	r := httptest.NewRequest(http.MethodPut, "/x", nil)
	r.Header.Add("If-Match", `"5"`)
	r.Header.Add("If-Match", `"6"`)
	if _, err := Precondition(r, true); FromError(err).HTTPStatus != 422 {
		t.Fatalf("two If-Match lines naming two revisions = %v", err)
	}
	r.Header.Set("If-Match", `W/"5"`)
	if pre, err := Precondition(r, true); err != nil || pre.Revision != 5 {
		t.Fatalf("weak If-Match = %+v %v", pre, err)
	}
	rec := httptest.NewRecorder()
	SetETag(rec, 4182)
	if got := rec.Header().Get("ETag"); got != `"4182"` || ETag(4182) != got {
		t.Fatalf("ETag %q", got)
	}
}

func TestSetPreconditionReadsBack(t *testing.T) {
	for _, pre := range []resource.Precondition{{}, {Revision: 4182}} {
		r := httptest.NewRequest(http.MethodPut, "/x", nil)
		SetPrecondition(r.Header, pre)
		got, err := Precondition(r, true)
		if err != nil || got != pre {
			t.Errorf("%+v read back as %+v %v", pre, got, err)
		}
	}
}
