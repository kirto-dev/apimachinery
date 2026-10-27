// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"encoding/json"
	"reflect"
	"testing"
)

// The zero Principal is what a handler sees when no authenticator ran.
// It has to read as nobody, or a forgotten middleware serves a request
// as somebody.
func TestTheZeroPrincipalIsAnonymous(t *testing.T) {
	var p Principal
	if !p.IsAnonymous() {
		t.Fatal("zero Principal is not anonymous")
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("zero Principal does not validate: %v", err)
	}
	if !(Principal{Kind: PrincipalAnonymous}).IsAnonymous() {
		t.Fatal("Anonymous() is not anonymous")
	}
	// Roles on a principal with no identity grant nothing to anybody:
	// the ID is the load-bearing field.
	if !(Principal{Roles: []string{"admin"}}).IsAnonymous() {
		t.Error("a Principal with roles and no id or kind should be anonymous")
	}
}

// A subject is unique only within its issuer, so the stable name pairs
// them, under the kind, and kinds without an issuer qualify by kind.
func TestQualifiedSeparatesWhatABareIDWouldMerge(t *testing.T) {
	cases := []struct {
		p    Principal
		want string
	}{
		{Principal{Kind: PrincipalHuman, Issuer: "corp", ID: "admin"}, "human:corp/admin"},
		{Principal{Kind: PrincipalHuman, Issuer: "github", ID: "admin"}, "human:github/admin"},
		{Principal{Kind: PrincipalWorkload, Issuer: "prod.example", ID: "ns/a/sa/b"}, "workload:prod.example/ns/a/sa/b"},
		{Principal{Kind: PrincipalSystem, ID: "admin", Name: "admin"}, "system:admin"},
		{Principal{Kind: PrincipalSystem}, "system"},
		{Principal{Kind: PrincipalInsecure, Name: "loopback"}, "insecure"},
		{Principal{Kind: PrincipalAnonymous}, "anonymous"},
		{Principal{}, "anonymous"},
	}
	seen := map[string]bool{}
	for _, tc := range cases {
		if got := tc.p.Qualified(); got != tc.want {
			t.Errorf("%+v.Qualified() = %q, want %q", tc.p, got, tc.want)
		}
		if tc.p.IsAnonymous() {
			continue
		}
		if seen[tc.want] {
			t.Errorf("%q names two principals", tc.want)
		}
		seen[tc.want] = true
	}
}

// Each pair once shared a qualified name, so a policy naming one was
// satisfied by the other: a different kind under one issuer name, and
// a separator moved between issuer and id.
func TestQualifiedIsInjective(t *testing.T) {
	pairs := [][2]Principal{
		{{Kind: PrincipalKey, Issuer: "unix", ID: "0"}, {Kind: PrincipalLocal, Issuer: "unix", ID: "0"}},
		{{Kind: PrincipalWorkload, Issuer: "corp", ID: "alice"}, {Kind: PrincipalHuman, Issuer: "corp", ID: "alice"}},
		{{Kind: PrincipalHuman, Issuer: "a/b", ID: "c"}, {Kind: PrincipalHuman, Issuer: "a", ID: "b/c"}},
		{{Kind: PrincipalHuman, Issuer: "a%2Fb", ID: "c"}, {Kind: PrincipalHuman, Issuer: "a/b", ID: "c"}},
		{{Kind: PrincipalSystem, ID: "a/b"}, {Kind: PrincipalSystem, Issuer: "a", ID: "b"}},
		{{Kind: PrincipalSystem, ID: "a%2Fb"}, {Kind: PrincipalSystem, ID: "a/b"}},
		{{Kind: PrincipalSystem, ID: "x"}, {Kind: PrincipalKey, ID: "x"}},
	}
	for _, pr := range pairs {
		if a, b := pr[0].Qualified(), pr[1].Qualified(); a == b {
			t.Errorf("%+v and %+v both qualify as %q", pr[0], pr[1], a)
		}
	}
}

func TestStringPrefersTheReadableName(t *testing.T) {
	p := Principal{Kind: PrincipalHuman, Issuer: "corp", ID: "00u1abc", Name: "ada@example.com"}
	if p.String() != "ada@example.com" {
		t.Errorf("String() = %q", p.String())
	}
	p.Name = ""
	if p.String() != "human:corp/00u1abc" {
		t.Errorf("String() without a name = %q", p.String())
	}
	if a := (Principal{Kind: PrincipalAnonymous, Name: "x"}); a.String() != "anonymous" {
		t.Errorf("anonymous prints as %q", a.String())
	}
}

func TestRolesGroupsAndClaims(t *testing.T) {
	p := Principal{
		Kind: PrincipalHuman, Issuer: "corp", ID: "u1",
		Roles:  []string{"admin", "auditor"},
		Groups: []string{"engineering", "oncall"},
		Claims: map[string]string{"email": "ada@example.com"},
	}
	if !p.HasRole("admin") || p.HasRole("manager") {
		t.Error("HasRole")
	}
	if !p.InGroup("oncall") || p.InGroup("sales") {
		t.Error("InGroup")
	}
	if p.Claim("email") != "ada@example.com" || p.Claim("missing") != "" {
		t.Error("Claim")
	}
}

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    Principal
		ok   bool
	}{
		{"human", Principal{Kind: PrincipalHuman, Issuer: "corp", ID: "u1"}, true},
		{"system without a name", Principal{Kind: PrincipalSystem}, true},
		{"insecure", Principal{Kind: PrincipalInsecure, Name: "dev"}, true},
		{"unknown kind", Principal{Kind: "oidc", ID: "u1"}, false},
		{"human without an id", Principal{Kind: PrincipalHuman, Issuer: "corp"}, false},
		{"local without an id", Principal{Kind: PrincipalLocal}, false},
		{"bad spiffe", Principal{Kind: PrincipalWorkload, ID: "x", SPIFFE: "https://x"}, false},
		{"spiffe outside the grammar", Principal{Kind: PrincipalWorkload, ID: "x", SPIFFE: "spiffe://PROD/../%2e"}, false},
		{"spiffe with a port", Principal{Kind: PrincipalWorkload, ID: "x", SPIFFE: "spiffe://prod.example:1/x"}, false},
		{"spiffe", Principal{Kind: PrincipalWorkload, ID: "x", SPIFFE: "spiffe://prod.example/x"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.p.Validate(); (err == nil) != tc.ok {
				t.Errorf("Validate() = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

// A Principal is copied into workflow inputs and audit records, so the
// JSON form has to round-trip whole.
func TestJSONRoundTrip(t *testing.T) {
	in := Principal{
		Kind: PrincipalWorkload, Issuer: "prod.example", ID: "ns/a/sa/b", Name: "b",
		Roles: []string{"r"}, Groups: []string{"g"},
		SPIFFE: "spiffe://prod.example/ns/a/sa/b",
		Claims: map[string]string{"k": "v"},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Principal
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip:\n in  %+v\n out %+v", in, out)
	}
	// The zero value encodes to nothing, so an anonymous starter adds
	// no noise to a stored input.
	if b, _ := json.Marshal(Principal{}); string(b) != "{}" {
		t.Errorf("zero Principal encodes as %s", b)
	}
}

func TestCloneSharesNothing(t *testing.T) {
	p := Principal{Kind: PrincipalHuman, ID: "u", Roles: []string{"a"}, Claims: map[string]string{"k": "v"}}
	c := p.Clone()
	c.Roles[0] = "b"
	c.Claims["k"] = "w"
	if p.Roles[0] != "a" || p.Claims["k"] != "v" {
		t.Error("Clone shares storage with the original")
	}
}
