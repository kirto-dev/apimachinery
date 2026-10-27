// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

// Package spiffeid is the SPIFFE ID grammar: what
// [kirto.dev/apimachinery/resource.Principal.Validate] holds a
// principal's SPIFFE field to, and what a server reading an mTLS peer's
// certificate or a JWT-SVID's subject parses with, so the two never
// disagree about which IDs exist.
package spiffeid

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Parse reads a SPIFFE ID from its URI form into a trust domain and path
// segments.
func Parse(raw string) (trustDomain string, segments []string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", nil, fmt.Errorf("spiffe: %q is not a URI: %w", raw, err)
	}
	return FromURL(u)
}

// FromURL reads a SPIFFE ID from a parsed URI, the shape a certificate's
// URIs field holds.
func FromURL(u *url.URL) (trustDomain string, segments []string, err error) {
	if u == nil {
		return "", nil, errors.New("spiffe: nil URI")
	}
	switch {
	case u.Scheme != "spiffe":
		return "", nil, fmt.Errorf("spiffe: %q is not a spiffe:// URI", u)
	case u.Opaque != "":
		return "", nil, fmt.Errorf("spiffe: %q is not spiffe://<trust-domain>/<path>", u)
	case u.User != nil, u.Port() != "":
		// The spec forbids both, and a port in particular would make
		// two spellings of one trust domain.
		return "", nil, fmt.Errorf("spiffe: %q carries a user or a port, which a SPIFFE ID may not", u)
	case u.RawQuery != "" || u.ForceQuery, u.Fragment != "":
		return "", nil, fmt.Errorf("spiffe: %q carries a query or a fragment, which a SPIFFE ID may not", u)
	}
	trustDomain = u.Host
	if p := u.EscapedPath(); p != "" {
		if !strings.HasPrefix(p, "/") {
			return "", nil, fmt.Errorf("spiffe: %q has a relative path", u)
		}
		segments = strings.Split(p[1:], "/")
	}
	if err := Check(trustDomain, segments); err != nil {
		return "", nil, fmt.Errorf("spiffe: %q: %w", u, err)
	}
	return trustDomain, segments, nil
}

// Check applies the SPIFFE ID grammar: a lowercase trust domain of
// letters, digits, dots, dashes and underscores, and path segments of
// letters, digits, dots, dashes and underscores that are neither empty
// nor a dot segment. A percent-escape is outside the grammar, so two
// spellings can never name one workload.
func Check(trustDomain string, segments []string) error {
	if err := CheckTrustDomain(trustDomain); err != nil {
		return err
	}
	for _, s := range segments {
		switch s {
		case "":
			return errors.New("empty path segment")
		case ".", "..":
			return fmt.Errorf("dot segment %q in path", s)
		}
		for _, r := range s {
			if !lowerAlnum(r) && !upper(r) && !punct(r) {
				return fmt.Errorf("path segment %q has %q, outside [a-zA-Z0-9._-]", s, r)
			}
		}
	}
	return nil
}

// CheckTrustDomain applies the trust-domain half of [Check].
func CheckTrustDomain(trustDomain string) error {
	if trustDomain == "" {
		return errors.New("no trust domain")
	}
	for _, r := range trustDomain {
		if !lowerAlnum(r) && !punct(r) {
			return fmt.Errorf("trust domain %q has %q, outside [a-z0-9._-]", trustDomain, r)
		}
	}
	return nil
}

func lowerAlnum(r rune) bool { return r >= 'a' && r <= 'z' || r >= '0' && r <= '9' }

func upper(r rune) bool { return r >= 'A' && r <= 'Z' }

func punct(r rune) bool { return r == '.' || r == '-' || r == '_' }
