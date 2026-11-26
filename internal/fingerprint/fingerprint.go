// Package fingerprint computes the short code that changes when, and only
// when, a record's meaning changes. The method is fixed precisely, because
// any drift would look like every record changing:
//
//  1. Take the record's meaning fields; "body" stands for the body.
//  2. Write them as compact JSON with sorted keys, in UTF-8.
//  3. Normalise the body: Windows line endings become Unix ones, trailing
//     spaces and tabs are removed from each line, runs of blank lines become
//     one, and blank lines at the start and end are dropped.
//  4. Hash the result with SHA-256 and keep the first 12 hex characters.
//
// Writing IDs without their @rN is the caller's job: this package knows
// nothing about records or IDs. The design is in
// docs/context-graph/03-implementation.md, section 5.
package fingerprint

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Method is the method this package computes. If it ever changes, it
// becomes method 2 (fp2:), and fingerprints of different methods are never
// compared.
const Method = 1

// FP is a fingerprint, written fp1:3a9c0e1b77d2.
type FP struct {
	Method int
	Hex    string
}

var fpRE = regexp.MustCompile(`^fp([1-9][0-9]*):([0-9a-f]{12})$`)

// Parse reads a fingerprint.
func Parse(s string) (FP, error) {
	m := fpRE.FindStringSubmatch(s)
	if m == nil {
		return FP{}, fmt.Errorf("%q isn't a fingerprint such as fp1:3a9c0e1b77d2", s)
	}
	method, _ := strconv.Atoi(m[1])
	return FP{Method: method, Hex: m[2]}, nil
}

func (f FP) String() string {
	if f.IsZero() {
		return ""
	}
	return fmt.Sprintf("fp%d:%s", f.Method, f.Hex)
}

// IsZero reports whether f is no fingerprint at all.
func (f FP) IsZero() bool { return f == FP{} }

// Of returns the fingerprint of a record's meaning: its meaning fields as
// plain values (text, numbers, lists, maps), and its body when the body is
// one of them. Pass hasBody false when it isn't.
func Of(fields map[string]any, body string, hasBody bool) FP {
	sum := sha256.Sum256(Canonical(fields, body, hasBody))
	return FP{Method: Method, Hex: hex.EncodeToString(sum[:])[:12]}
}

// Canonical is the text that is hashed.
func Canonical(fields map[string]any, body string, hasBody bool) []byte {
	all := make(map[string]any, len(fields)+1)
	for k, v := range fields {
		all[k] = v
	}
	if hasBody {
		all["body"] = NormalizeBody(body)
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false) // keep <, > and & as written
	if err := enc.Encode(all); err != nil {
		// Plain values from a parsed header always encode.
		panic(fmt.Sprintf("fingerprint: %v", err))
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

// NormalizeBody removes the differences in a body that don't change what it
// says.
func NormalizeBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	var out []string
	blank := false
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			blank = len(out) > 0
			continue
		}
		if blank {
			out = append(out, "")
			blank = false
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
