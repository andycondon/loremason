// Package id makes and takes apart record IDs.
package id

import (
	"crypto/rand"
	"crypto/sha256"
	"regexp"
)

// alphabet is digits and lowercase letters without i, l, o and u.
const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

var idRE = regexp.MustCompile(`^([a-z]+)_[0-9a-hjkmnp-tv-z]{6}$`)

// New returns a fresh random ID with the given prefix.
func New(prefix string) string {
	var b [6]byte
	rand.Read(b[:]) // never fails: crypto/rand aborts the program instead
	for i := range b {
		b[i] = alphabet[b[i]%32]
	}
	return prefix + "_" + string(b[:])
}

// Prefix returns the type prefix of a record ID, and whether s is one.
func Prefix(s string) (string, bool) {
	m := idRE.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// Derive returns the ID with the given prefix that key always gives, so the
// same key names the same record without a lookup. Different keys can share
// an ID, but only by chance.
func Derive(prefix, key string) string {
	sum := sha256.Sum256([]byte(key))
	b := sum[:6]
	for i := range b {
		b[i] = alphabet[b[i]%32]
	}
	return prefix + "_" + string(b)
}
