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

// ParseRef reads a reference. The schemas' `ref` pattern accepts exactly the
// same text, so a link the schema check passes always parses.
func ParseRef(s string) (Ref, error) {
	if s == "" || strings.ContainsAny(s, " \t\n\f\r") {
		return Ref{}, errors.New("a reference can't be empty or contain spaces")
	}
	i := strings.LastIndexByte(s, '@')
	if i < 0 {
		return Ref{Name: s}, nil
	}
	name, rev := s[:i], s[i+1:]
	if name == "" || strings.Contains(name, "@") || !revRE.MatchString(rev) {
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

// SubrecordID returns a sub-record's ID: its record's ID, a slash, and its
// local id, as in bel_7k3m9q/s1.
func SubrecordID(record, local string) string { return record + "/" + local }

// splitSubrecord splits a name at its last slash into the record part and
// the local part. ok is false when there's no slash.
func splitSubrecord(name string) (record, local string, ok bool) {
	i := strings.LastIndexByte(name, '/')
	if i < 0 {
		return name, "", false
	}
	return name[:i], name[i+1:], true
}
