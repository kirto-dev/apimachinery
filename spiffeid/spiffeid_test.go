// Copyright Kirto LLC 2026
// SPDX-License-Identifier: Apache-2.0

package spiffeid

import (
	"slices"
	"testing"
)

func TestParse(t *testing.T) {
	td, segs, err := Parse("spiffe://prod.example/ns/a/sa/b")
	if err != nil || td != "prod.example" || !slices.Equal(segs, []string{"ns", "a", "sa", "b"}) {
		t.Errorf("Parse = %q %q %v", td, segs, err)
	}
	if td, segs, err := Parse("spiffe://prod.example"); err != nil || td != "prod.example" || segs != nil {
		t.Errorf("trust domain alone = %q %q %v", td, segs, err)
	}
	for _, bad := range []string{
		"spiffe://PROD/../%2e",
		"spiffe://prod.example/a/../b",
		"spiffe://prod.example/a//b",
		"spiffe://prod.example/a%2Fb",
		"spiffe:///x",
		"spiffe://prod.example:443/x",
		"spiffe://u@prod.example/x",
		"spiffe://prod.example/x?y",
		"spiffe://prod.example/x#y",
		"spiffe:prod.example/x",
		"https://prod.example/x",
		"spiffe://prod example/x",
	} {
		if _, _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) accepted", bad)
		}
	}
}
