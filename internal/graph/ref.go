package graph

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Ref is a reference as a link writes it: a record's ID or one of its
// aliases, then /local for a sub-record, then @rN for a revision. For example
// bel_7k3m9q, bel_7k3m9q/s1, plan_k2x8wd@r2.
//
// Name is everything before the revision. Which part of it names a
// sub-record is only known by resolving it, because an alias may itself
// contain a slash; see Graph.Resolve.
type Ref struct {
	Name string
	Rev  int // 0 when no revision is named
}

var revRE = regexp.MustCompile(`^r[1-9][0-9]{0,8}$`)

// ParseRef reads a reference.
func ParseRef(s string) (Ref, error) {
	if s == "" || strings.ContainsFunc(s, isSpace) {
		return Ref{}, errors.New("a reference can't be empty or contain spaces")
	}
	name, rev, found := cutLast(s, "@")
	if !found {
		return Ref{Name: s}, nil
	}
	if name == "" || !revRE.MatchString(rev) {
		return Ref{}, fmt.Errorf("`%s` isn't a reference: a revision goes last and counts from 1, as in bel_7k3m9q@r2 or bel_7k3m9q/s1@r2", s)
	}
	n, _ := strconv.Atoi(rev[1:])
	return Ref{Name: name, Rev: n}, nil
}

func (r Ref) String() string {
	if r.Rev == 0 {
		return r.Name
	}
	return fmt.Sprintf("%s@r%d", r.Name, r.Rev)
}

func cutLast(s, sep string) (before, after string, found bool) {
	if i := strings.LastIndex(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

func isSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\r' || r == '\n' }
