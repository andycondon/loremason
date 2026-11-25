// Package id makes and takes apart record IDs.
package id

import (
	"crypto/rand"
	"regexp"
	"strings"
)

// alphabet is digits and lowercase letters without i, l, o and u.
const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

var (
	idRE  = regexp.MustCompile(`^([a-z]+)_[0-9a-hjkmnp-tv-z]{6}$`)
	revRE = regexp.MustCompile(`@r[1-9][0-9]*$`)
)

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

// TrimRevision removes a revision suffix such as @r2.
func TrimRevision(s string) string {
	return s[:len(s)-len(revRE.FindString(s))]
}

// SplitSub splits bel_7k3m9q/s1 at its last slash. ok is false without one.
func SplitSub(s string) (parent, local string, ok bool) {
	i := strings.LastIndexByte(s, '/')
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+1:], true
}
