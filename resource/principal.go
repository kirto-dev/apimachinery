// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"kirto.dev/apimachinery/spiffeid"
)

// Principal is one authenticated caller: who it is and what an issuer
// assigned it, never what it may do. A server builds one from the
// credential that admitted the caller; a client reads one back, in a
// case's starter or a session, in the same JSON.
//
// The zero value is anonymous, so a request that reached a handler
// without passing an authenticator is denied wherever a policy is
// configured rather than served as somebody.
//
// A Principal is a value: copy it into a workflow input or an audit
// record and the copy is the identity. The JSON form is stable for that
// reason.
//
// # Identity is issuer-qualified
//
// A subject is unique only within the issuer that assigned it. Two
// directories may both have a user "admin", and two trust domains may
// both have a workload "node/1". So the stable name of a caller is the
// kind, the issuer and the subject together, [Principal.Qualified], and
// a policy written over a bare ID is a policy that one issuer can
// satisfy on behalf of another.
//
// The kind is in the name because issuer names are configuration, and
// two mechanisms can vouch under one name: a local peer under "unix" and
// an OIDC issuer an operator also called "unix". The kind keeps those
// apart. It does not keep apart two workload mechanisms under one name,
// so a SPIFFE trust domain and an OIDC issuer that marks its callers as
// workloads must not share a name in one deployment.
type Principal struct {
	// Kind is how the caller was identified.
	Kind PrincipalKind `json:"kind,omitempty"`
	// Issuer names who vouched for the caller: the configured name of an
	// OIDC issuer, a SPIFFE trust domain, "unix" for a local peer, or the
	// product's own name for a key it minted. Empty for kinds that have
	// no voucher (anonymous, insecure, system).
	Issuer string `json:"issuer,omitempty"`
	// ID is the stable subject, unique within Issuer. It is often
	// unreadable (a directory's opaque id, a uid, a SPIFFE path) and it
	// is what a policy names. Never an email or a display name: those
	// are mutable and often set by the caller.
	ID string `json:"id,omitempty"`
	// Name is what a person reading a trail recognizes. It is printed,
	// never decided on.
	Name string `json:"name,omitempty"`
	// Roles are the roles an issuer assigned, sorted and deduplicated by
	// the mechanism that built the Principal. What a role permits is a
	// product's policy, not this package's.
	Roles []string `json:"roles,omitempty"`
	// Groups are the groups an issuer reports the caller in, kept apart
	// from Roles because a product may assign work to a group without
	// granting anything by it.
	Groups []string `json:"groups,omitempty"`
	// SPIFFE is the caller's SPIFFE ID (spiffe://<trust-domain>/<path>)
	// where it has one: an mTLS peer, or a JWT-SVID whose subject is one.
	SPIFFE string `json:"spiffe,omitempty"`
	// Claims are a few issuer-assigned attributes worth carrying beside
	// the identity: an email for a notification, a pid for the trail. It
	// is a small string map rather than the token's claim set so that a
	// Principal has one canonical encoding. Nothing in it is an
	// identity key, and no policy reads it: a mechanism carries claims
	// without the checks that guard roles, so the caller may have set
	// them.
	Claims map[string]string `json:"claims,omitempty"`
}

// IsAnonymous reports whether p has no resolved identity: the anonymous
// kind, or a zero Principal that no authenticator touched.
func (p Principal) IsAnonymous() bool {
	return p.Kind == PrincipalAnonymous || (p.Kind == "" && p.ID == "")
}

// Qualified is the stable, globally unique name of the caller:
// "<kind>:<issuer>/<id>", or "<kind>:<id>" for a kind with no issuer,
// "<kind>" with neither, and "anonymous" for every anonymous caller.
// It is the [Writer.Principal] of what the caller writes.
//
// It is injective over Principals that differ in kind, issuer or id.
// The kind ends at the first colon, since no kind contains one. A "/"
// or "%" inside the issuer is written %2F or %25, so the first "/"
// ends the issuer. With no issuer, the same escape applies to the id,
// so a name with no unescaped "/" is one with no issuer. The id after
// an issuer is written as it is, because SPIFFE paths and workload
// subjects carry slashes and a policy names them as they read.
func (p Principal) Qualified() string {
	switch {
	case p.IsAnonymous():
		return string(PrincipalAnonymous)
	case p.Issuer != "":
		return string(p.Kind) + ":" + escapeQualified(p.Issuer) + "/" + p.ID
	case p.ID != "":
		return string(p.Kind) + ":" + escapeQualified(p.ID)
	}
	return string(p.Kind)
}

var qualifiedEscaper = strings.NewReplacer("%", "%25", "/", "%2F")

func escapeQualified(s string) string { return qualifiedEscaper.Replace(s) }

// String is the readable form for a log line or an error: the name
// where there is one, and otherwise the qualified identity.
func (p Principal) String() string {
	if p.Name != "" && !p.IsAnonymous() {
		return p.Name
	}
	return p.Qualified()
}

// HasRole reports whether p carries the role.
func (p Principal) HasRole(role string) bool { return slices.Contains(p.Roles, role) }

// InGroup reports whether p is a member of the group.
func (p Principal) InGroup(group string) bool { return slices.Contains(p.Groups, group) }

// Claim returns one of p's carried claims, or empty.
func (p Principal) Claim(name string) string { return p.Claims[name] }

// Validate reports a Principal no authenticator would build: an unknown
// kind, an identified kind with no subject, or a SPIFFE field outside
// the SPIFFE ID grammar. A server calls it on a Principal it decoded
// from somewhere it did not mint, such as a workflow input.
func (p Principal) Validate() error {
	if p.Kind == "" && p.IsAnonymous() {
		return nil
	}
	if !p.Kind.Valid() {
		return fmt.Errorf("identity: unknown principal kind %q", p.Kind)
	}
	switch p.Kind {
	case PrincipalHuman, PrincipalWorkload, PrincipalNode, PrincipalKey, PrincipalLocal:
		if p.ID == "" {
			return fmt.Errorf("identity: a %s principal needs an id", p.Kind)
		}
	}
	if p.SPIFFE != "" {
		if _, _, err := spiffeid.Parse(p.SPIFFE); err != nil {
			return fmt.Errorf("identity: spiffe %q is not a SPIFFE ID: %w", p.SPIFFE, err)
		}
	}
	return nil
}

// Clone returns a copy that shares no slice or map with p, for a caller
// that will hand it to code it does not control.
func (p Principal) Clone() Principal {
	p.Roles = slices.Clone(p.Roles)
	p.Groups = slices.Clone(p.Groups)
	p.Claims = maps.Clone(p.Claims)
	return p
}
