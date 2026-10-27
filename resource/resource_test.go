// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package resource_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"kirto.dev/apimachinery/resource"
)

func TestCursorRoundTrip(t *testing.T) {
	c := resource.FormatCursor(4190, "9f2c")
	if c != "4190.9f2c" {
		t.Fatalf("cursor %q", c)
	}
	rev, epoch, err := resource.ParseCursor(c)
	if err != nil || rev != 4190 || epoch != "9f2c" {
		t.Fatalf("parse %q = %d %q %v", c, rev, epoch, err)
	}
	// The epoch is everything after the first dot.
	if rev, epoch, err := resource.ParseCursor("7.a.b"); err != nil || rev != 7 || epoch != "a.b" {
		t.Fatalf("parse 7.a.b = %d %q %v", rev, epoch, err)
	}
	for _, s := range []string{"", "0"} {
		if rev, epoch, err := resource.ParseCursor(s); err != nil || rev != 0 || epoch != "" {
			t.Fatalf("parse %q = %d %q %v, want from 0", s, rev, epoch, err)
		}
	}
	for _, s := range []string{"4182", "4182.", ".abc", "x.abc", "-1.abc", "1 .abc"} {
		if _, _, err := resource.ParseCursor(s); !errors.Is(err, resource.ErrInvalid) {
			t.Errorf("parse %q = %v, want invalid", s, err)
		}
	}
}

func TestStatusOfForeignError(t *testing.T) {
	if _, ok := resource.StatusOf(errors.New("other")); ok {
		t.Fatal("a foreign error has a status")
	}
	if _, ok := resource.StatusOf(nil); ok {
		t.Fatal("nil has a status")
	}
	st := resource.Status{Code: "later_code", Message: "from a newer node"}
	if err := st.Err(); err.Error() != "from a newer node" {
		t.Fatalf("unknown code: %v", err)
	}
}

// Every typed error survives the trip through Status with each field
// it carries, so a refusal that crossed a node is the same error.
func TestStatusRoundTrip(t *testing.T) {
	k := resource.Key{Kind: "item", Namespace: "n", Name: "a"}
	for _, err := range []error{
		&resource.NotFoundError{Key: k},
		&resource.NotFoundError{Set: "s"},
		&resource.ConflictError{Key: k, Expected: 3, Current: 4},
		&resource.ConflictError{Expected: 3, Current: 9, Store: true},
		&resource.SetConflictError{Conflicts: []resource.SetConflict{{Key: k, Holder: resource.Membership{Name: "api", Mode: resource.SetImperative}, Revision: 7}}},
		&resource.TooOldError{Revision: 2, Compacted: 5},
		&resource.TooOldError{Revision: 2, Epoch: "e"},
		&resource.InvalidNameError{Field: "name", Value: "", Reason: "a name is required"},
		&resource.InvalidError{Key: k, Reason: "no"},
		resource.Errorf(resource.ErrQuota, "the store holds its limit of %d objects", 3),
	} {
		s, ok := resource.StatusOf(fmt.Errorf("wrapped: %w", err))
		if !ok {
			t.Fatalf("%v has no status", err)
		}
		b, _ := json.Marshal(s)
		var back resource.Status
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatal(err)
		}
		got := back.Err()
		if s2, _ := resource.StatusOf(got); s2.Code != s.Code {
			t.Errorf("%v: code %s, back %s", err, s.Code, s2.Code)
		}
		for _, sentinel := range []error{resource.ErrNotFound, resource.ErrConflict, resource.ErrSetConflict, resource.ErrTooOld,
			resource.ErrInvalidName, resource.ErrInvalid, resource.ErrQuota} {
			if errors.Is(err, sentinel) != errors.Is(got, sentinel) {
				t.Errorf("%v: errors.Is %v differs after the trip (%v)", err, sentinel, got)
			}
		}
		// A typed error rebuilds its message from its fields; a sentinel
		// with a message keeps the whole text, wrapping included.
		want := err.Error()
		if s.Code == resource.CodeQuota {
			want = s.Message
		}
		if got.Error() != want {
			t.Errorf("message %q, back %q", want, got.Error())
		}
	}
}

func TestErrorfKeepsItsOwnMessage(t *testing.T) {
	err := resource.Errorf(resource.ErrTooLarge, "%s is %d bytes", "a", 9)
	if !errors.Is(err, resource.ErrTooLarge) || err.Error() != "a is 9 bytes" {
		t.Fatalf("%v", err)
	}
}

func TestNames(t *testing.T) {
	for _, c := range []struct {
		fn   func(string) error
		ok   []string
		fail []string
	}{
		{resource.ValidKind, []string{"a", "model_ref", "x-1"}, []string{"", "A", "1a", strings.Repeat("a", 64)}},
		{resource.ValidNamespace, []string{"a", "team-1"}, []string{"", "-a", "a-", "A", strings.Repeat("a", 65)}},
		{resource.ValidSetName, []string{"api"}, []string{"", "a_b"}},
		{resource.ValidName, []string{"a", "v1.2/x@y", "ü"}, []string{"", " a", "a ", "a\x00b", strings.Repeat("a", resource.MaxNameBytes+1), "\xff"}},
	} {
		for _, s := range c.ok {
			if err := c.fn(s); err != nil {
				t.Errorf("%q: %v", s, err)
			}
		}
		for _, s := range c.fail {
			if err := c.fn(s); !errors.Is(err, resource.ErrInvalidName) {
				t.Errorf("%q = %v, want invalid name", s, err)
			}
		}
	}
}

func TestEveryPrincipalKindIsValid(t *testing.T) {
	for _, k := range resource.PrincipalKinds() {
		if !k.Valid() {
			t.Errorf("%q is listed but not valid", k)
		}
	}
	if resource.PrincipalKind("").Valid() {
		t.Error("the empty kind is valid")
	}
}

func TestBlobRef(t *testing.T) {
	good := resource.BlobRef{Digest: "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", Size: 5}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	if good.Hex() != strings.TrimPrefix(good.Digest, "sha256:") {
		t.Errorf("Hex = %q", good.Hex())
	}
	upper := resource.BlobRef{Digest: "sha256:" + strings.ToUpper(good.Hex())}
	for _, bad := range []resource.BlobRef{
		{Digest: "md5:abc"},
		{Digest: "sha256:" + good.Hex()[:63]},
		{Digest: "sha256:" + good.Hex()[:63] + "g"},
		upper,
		{Digest: good.Digest, Size: -1},
	} {
		err := bad.Validate()
		if !errors.Is(err, resource.ErrInvalidBlobRef) {
			t.Errorf("Validate(%+v) = %v, want ErrInvalidBlobRef", bad, err)
		}
		if err != nil && strings.HasPrefix(err.Error(), "blob:") {
			t.Errorf("Validate message %q carries the package prefix", err)
		}
	}
	if upper.Hex() != "" {
		t.Errorf("Hex of an uppercase digest = %q", upper.Hex())
	}
}

func TestMetaJSON(t *testing.T) {
	b, err := json.Marshal(resource.Meta{Name: "a", ResourceVersion: "4", Writer: &resource.Writer{Principal: "p", Kind: resource.PrincipalHuman}})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"name":"a","resourceVersion":"4","writer":{"principal":"p","kind":"human"}}`; string(b) != want {
		t.Fatalf("%s, want %s", b, want)
	}
}
