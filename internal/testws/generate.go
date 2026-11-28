package testws

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"

	"github.com/andycondon/loremason/internal/fingerprint"
	"github.com/andycondon/loremason/internal/graph"
)

// Generate writes a valid workspace of about n records into dir, the same
// files every time for a given seed. It's for tests that need a workspace
// of realistic size, such as a speed test on thousands of records.
//
// The records are spread across every record type. They link to each other
// and pin what they rely on, and beliefs and requirements have sub-records
// or revisions. A few pins are stale, because the belief behind them has
// been revised; a few beliefs have fallen (incorrect or insufficient) and
// decisions and tasks rely on them; one belief is withdrawn. Titles and
// bodies are made of varied words, so a full-text search has things to find.
func Generate(dir string, n int, seed int64) error {
	g := &generator{rng: rand.New(rand.NewPCG(uint64(seed), 0x6c6f72656d61736f)), ids: map[string]bool{}, texts: map[string]bool{}, files: map[string]string{}}
	g.build(n)
	if err := os.MkdirAll(filepath.Join(dir, ".loremason"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, ".loremason", "config.yaml"), []byte("version: "+Version+"\n"), 0o644); err != nil {
		return err
	}
	made := map[string]bool{}
	for rel, text := range g.files {
		p := filepath.Join(dir, graph.Dir, filepath.FromSlash(rel))
		if folder := filepath.Dir(p); !made[folder] {
			if err := os.MkdirAll(folder, 0o755); err != nil {
				return err
			}
			made[folder] = true
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			return err
		}
	}
	return nil
}

type generator struct {
	rng   *rand.Rand
	ids   map[string]bool
	texts map[string]bool   // text that must be unique, by kind and text
	files map[string]string // record text by path under graph/
}

// belief is what a pin to a belief needs to know.
type belief struct {
	id     string
	rev    int            // the current revision
	fp     map[int]string // the fingerprint of each revision
	fallen bool
}

// commitment is a record that relies on beliefs.
type commitment struct {
	id     string
	relies []string // link texts, as written in relies_on
}

const idAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// shares is each type's part of the records, in percent; the shares sum to 100.
var shares = []struct {
	kind string
	pct  int
}{
	{"actor", 3}, {"artifact", 4}, {"activity", 4}, {"evidence", 14}, {"belief", 14},
	{"finding", 6}, {"question", 5}, {"initiative", 1}, {"prd", 3}, {"requirement", 8},
	{"decision", 7}, {"design_doc", 4}, {"task", 12}, {"plan", 4}, {"step", 11},
}

func (g *generator) build(n int) {
	count := map[string]int{}
	for _, s := range shares {
		count[s.kind] = max(1, n*s.pct/100)
	}
	count["belief"] = max(count["belief"], 6) // enough for the fallen and the revised
	count["decision"] = max(count["decision"], 2)
	count["task"] = max(count["task"], 2)

	actors := g.many(count["actor"], "who", func(id string) string { return g.actor(id) })
	arts := g.many(count["artifact"], "art", func(id string) string { return g.artifact(id, actors) })
	inis := g.many(count["initiative"], "ini", func(id string) string { return g.initiative(id, actors) })
	acts := g.many(count["activity"], "actv", func(id string) string { return g.activity(id, actors, inis) })
	beliefs, evs := g.beliefs(count["belief"], count["evidence"], arts, actors, acts)
	_ = evs
	g.many(count["finding"], "fnd", func(id string) string { return g.finding(id, acts, beliefs) })
	qs, opts := g.questions(count["question"], inis, beliefs)

	var commits []*commitment
	add := func(id string) *commitment {
		c := &commitment{id: id}
		commits = append(commits, c)
		return c
	}
	var decs, tasks []*commitment
	prds := g.many(count["prd"], "prd", func(id string) string { add(id); return g.prd(id, inis) })
	reqs, reqFP := g.requirements(count["requirement"], prds, add)
	dds := g.many(count["design_doc"], "dd", func(id string) string { add(id); return g.designDoc(id, inis, prds, reqs) })
	decIDs := g.many(count["decision"], "dec", func(id string) string {
		decs = append(decs, add(id))
		return g.decision(id, inis, qs, opts, actors)
	})
	taskIDs := g.many(count["task"], "task", func(id string) string {
		tasks = append(tasks, add(id))
		return g.task(id, inis, reqs, reqFP, dds)
	})
	_ = decIDs
	_ = taskIDs
	plans := g.many(count["plan"], "plan", func(id string) string { add(id); return g.plan(id, inis, taskIDs) })
	g.steps(count["step"], plans)

	g.relyOnBeliefs(commits, decs, tasks, beliefs)
	g.writeCommitments(commits)
}

// many makes k records with fresh IDs, and returns the IDs.
func (g *generator) many(k int, prefix string, text func(id string) string) []string {
	ids := make([]string, k)
	for i := range ids {
		ids[i] = g.newID(prefix)
		g.put(prefix, ids[i], text(ids[i]))
	}
	return ids
}

// folders maps an ID prefix to the folder under graph/.
var folders = map[string]string{
	"who": "actor", "art": "artifact", "actv": "activity", "ev": "evidence", "bel": "belief",
	"fnd": "finding", "q": "question", "ini": "initiative", "prd": "prd", "req": "requirement",
	"dec": "decision", "dd": "design-doc", "task": "task", "plan": "plan", "step": "step",
}

func (g *generator) put(prefix, id, text string) { g.files[folders[prefix]+"/"+id+".md"] = text }

func (g *generator) newID(prefix string) string {
	for {
		var b [6]byte
		for i := range b {
			b[i] = idAlphabet[g.rng.IntN(len(idAlphabet))]
		}
		id := prefix + "_" + string(b[:])
		if !g.ids[id] {
			g.ids[id] = true
			return id
		}
	}
}

// ---- words ----

var vocabulary = strings.Fields(`
	adapter archive audit backlog baseline binary bridge budget buffer cache cadence callback channel checksum
	cluster cohort compiler config container context cursor dashboard deadline decoder default dependency
	deploy digest dispatch domain encoder endpoint failure fallback filter fixture gateway handler heuristic
	hotfix import index ingest invariant journal kernel latency ledger lineage lookup manifest migration
	monitor namespace network observer offset operator outage packet parser payload pipeline plugin pointer
	policy profile protocol quorum queue reader record registry release replica request retry rollout router
	runtime sampler scanner schema scheduler segment session shard signal snapshot socket storage stream
	switch tenant throttle timeout token tracer transport trigger upgrade validator vendor version warehouse
	watcher worker workflow writer`)

var verbs = strings.Fields(`reads writes drops delays rejects retries merges splits caches replays
	truncates reorders blocks leaks duplicates skips recovers reports`)

// text returns k words, each from the vocabulary.
func (g *generator) text(k int) string {
	w := make([]string, k)
	for i := range w {
		w[i] = vocabulary[g.rng.IntN(len(vocabulary))]
	}
	return strings.Join(w, " ")
}

// unique returns a phrase of k or more words that no earlier one of the same
// kind used, for fields that mark two records as the same thing.
func (g *generator) unique(kind string, k int) string {
	for i := 0; ; i++ {
		s := g.text(k + i/20)
		if !g.texts[kind+"\x00"+s] {
			g.texts[kind+"\x00"+s] = true
			return s
		}
	}
}

// sentence is a sentence about some things: "the parser drops a cache".
func (g *generator) sentence() string {
	return fmt.Sprintf("The %s %s a %s %s.", g.text(1), verbs[g.rng.IntN(len(verbs))], g.text(1), g.text(1))
}

func (g *generator) paragraph() string {
	s := make([]string, 1+g.rng.IntN(4))
	for i := range s {
		s[i] = g.sentence()
	}
	return strings.Join(s, " ")
}

func (g *generator) date() string {
	return fmt.Sprintf("2026-%02d-%02d", 8+g.rng.IntN(4), 1+g.rng.IntN(28))
}

func (g *generator) pick(ids []string) string { return ids[g.rng.IntN(len(ids))] }

// some returns up to max distinct IDs, at least one.
func (g *generator) some(ids []string, max int) []string {
	k := min(1+g.rng.IntN(max), len(ids))
	var out []string
	for _, i := range g.rng.Perm(len(ids))[:k] {
		out = append(out, ids[i])
	}
	return out
}

func list(ids []string) string { return "[" + strings.Join(ids, ", ") + "]" }

// record writes a record: the header lines, then a body.
func record(id, title string, header []string, body string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\nid: %s\ntitle: %q\n", id, title)
	for _, l := range header {
		b.WriteString(l + "\n")
	}
	b.WriteString("---\n")
	if body != "" {
		b.WriteString("\n" + body + "\n")
	}
	return b.String()
}

var maturities = []string{"proposed", "leaning", "accepted", "rejected", "deprecated", "superseded"}

func (g *generator) maturity() string { return "status: " + maturities[g.rng.IntN(len(maturities))] }

// ---- knowing layer ----

func (g *generator) actor(id string) string {
	kind := []string{"person", "organization", "software_agent"}[g.rng.IntN(3)]
	return record(id, g.unique("actor", 2), []string{"kind: " + kind, "created: " + g.date()}, "")
}

func (g *generator) artifact(id string, actors []string) string {
	kind := []string{"page", "code", "conversation", "issue"}[g.rng.IntN(4)]
	h := []string{"kind: " + kind, "url: https://example.com/" + id, "attributed_to: " + g.pick(actors), "created: " + g.date()}
	return record(id, g.unique("artifact", 3), h, "")
}

func (g *generator) initiative(id string, actors []string) string {
	slug := strings.ReplaceAll(g.unique("slug", 2), " ", "-")
	h := []string{"slug: " + slug, "status: active", fmt.Sprintf("purpose: %q", g.text(6)), "goals:", fmt.Sprintf("  - %q", g.text(4)),
		"involves:", "  - {to: " + g.pick(actors) + ", role: decides}", "created: " + g.date()}
	return record(id, g.unique("initiative", 3), h, g.paragraph())
}

func (g *generator) activity(id string, actors, inis []string) string {
	day := g.date()
	h := []string{"started_at: " + day + "T09:00:00Z", "ended_at: " + day + "T10:30:00Z", "status: completed",
		"associated_with: " + g.pick(actors), "part_of: " + g.pick(inis), "created: " + day}
	return record(id, g.unique("activity", 3), h, "")
}

// beliefs writes the beliefs and the evidence they cite. Some beliefs have
// fallen, some are revised past an earlier revision, one is withdrawn.
func (g *generator) beliefs(nBeliefs, nEvidence int, arts, actors, acts []string) ([]*belief, []string) {
	evs := g.many(nEvidence, "ev", func(id string) string {
		from := g.pick(arts)
		if g.rng.IntN(5) == 0 {
			from = g.pick(actors)
		}
		h := []string{"observed_at: " + g.date(), "from: " + from, "generated_by: " + g.pick(acts), "created: " + g.date()}
		return record(id, g.unique("evidence", 4), h, "> "+g.paragraph())
	})
	fallen := max(2, nBeliefs/10)
	revised := max(2, nBeliefs/10)
	var out []*belief
	for i := 0; i < nBeliefs; i++ {
		b := &belief{id: g.newID("bel"), rev: 1, fp: map[int]string{}}
		status := "held"
		switch {
		case i < fallen:
			status, b.fallen = []string{"incorrect", "insufficient"}[i%2], true
		case i == fallen:
			status = "withdrawn"
		case i < fallen+1+revised:
			b.rev = 2
		}
		claim := g.unique("claim", 5)
		h := []string{"claim: " + fmt.Sprintf("%q", claim), "status: " + status, "asserted_by: " + []string{"person", "agent"}[g.rng.IntN(2)], "support:"}
		for s, ev := range g.some(evs, 2) {
			h = append(h, fmt.Sprintf("  - {id: s%d, evidence: %s, direction: %s}", s+1, ev, []string{"supports", "disputes"}[g.rng.IntN(2)]))
		}
		h = append(h, "revisions:")
		day := g.date()
		if b.rev == 2 {
			old := g.unique("claim", 5)
			b.fp[1] = claimFP(old)
			h = append(h, fmt.Sprintf("  - {rev: 1, date: %s, fp: %q, claim: %q}", day, b.fp[1], old))
			h = append(h, fmt.Sprintf("  - {rev: 2, date: %s, fp: %q, change: breaking}", day, claimFP(claim)))
			b.fp[2] = claimFP(claim)
		} else {
			b.fp[1] = claimFP(claim)
			h = append(h, fmt.Sprintf("  - {rev: 1, date: %s, fp: %q}", day, b.fp[1]))
		}
		h = append(h, "created: "+day)
		g.put("bel", b.id, record(b.id, claim, h, g.paragraph()))
		out = append(out, b)
	}
	return out, evs
}

func claimFP(claim string) string {
	return fingerprint.Of(map[string]any{"claim": claim}, "", false).String()
}

func (g *generator) finding(id string, acts []string, beliefs []*belief) string {
	h := []string{"generated_by: " + g.pick(acts), "mentions: [" + g.beliefs1(beliefs) + "]", "created: " + g.date()}
	return record(id, g.unique("finding", 4), h, g.paragraph())
}

func (g *generator) beliefs1(beliefs []*belief) string { return beliefs[g.rng.IntN(len(beliefs))].id }

func (g *generator) questions(n int, inis []string, beliefs []*belief) (qs, opts []string) {
	g.many(n, "q", func(id string) string {
		q := g.unique("question", 4) + "?"
		qs = append(qs, id)
		opts = append(opts, id+"/o1")
		h := []string{fmt.Sprintf("question: %q", q), "part_of: " + g.pick(inis), "questions: " + g.beliefs1(beliefs),
			"options:", fmt.Sprintf("  - {id: o1, title: %q, for: [%s]}", g.text(3), g.beliefs1(beliefs)),
			fmt.Sprintf("  - {id: o2, title: %q, against: [%s]}", g.text(3), g.beliefs1(beliefs)), "created: " + g.date()}
		return record(id, q, h, "")
	})
	return qs, opts
}

// ---- committing layer ----

func (g *generator) prd(id string, inis []string) string {
	return record(id, g.unique("prd", 4), []string{g.maturity(), "part_of: " + g.pick(inis), "created: " + g.date()}, g.paragraph())
}

// requirements writes requirements, each with its fingerprint recorded as
// revision 1, and returns their IDs and fingerprints.
func (g *generator) requirements(n int, prds []string, add func(string) *commitment) ([]string, map[string]string) {
	fps := map[string]string{}
	ids := g.many(n, "req", func(id string) string {
		add(id)
		stmt := g.unique("statement", 5)
		fps[id] = fingerprint.Of(map[string]any{"statement": stmt}, "", false).String()
		day := g.date()
		h := []string{fmt.Sprintf("statement: %q", stmt), g.maturity(), "part_of: " + g.pick(prds),
			"revisions:", fmt.Sprintf("  - {rev: 1, date: %s, fp: %q}", day, fps[id]), "created: " + day}
		return record(id, stmt, h, "")
	})
	return ids, fps
}

func (g *generator) designDoc(id string, inis, prds, reqs []string) string {
	h := []string{g.maturity(), "part_of: " + g.pick(inis), "elaborates: " + list([]string{g.pick(prds), g.pick(reqs)}), "created: " + g.date()}
	return record(id, g.unique("design_doc", 4), h, g.paragraph())
}

func (g *generator) decision(id string, inis, qs, opts, actors []string) string {
	i := g.rng.IntN(len(qs))
	h := []string{g.maturity(), "part_of: " + g.pick(inis), "answers: " + qs[i], "chooses: " + opts[i], "decided_by: " + g.pick(actors),
		"criteria:", fmt.Sprintf("  - %q", g.text(4)), fmt.Sprintf("confirmation: %q", g.text(5)), "created: " + g.date()}
	return record(id, g.unique("decision", 4), h, g.paragraph())
}

func (g *generator) task(id string, inis, reqs []string, reqFP map[string]string, dds []string) string {
	req := g.pick(reqs)
	h := []string{g.maturity(), "part_of: " + g.pick(inis), "implements:", fmt.Sprintf("  - {to: %s, rev: 1, fp: %q}", req, reqFP[req]),
		"derived_from: " + g.pick(dds), "created: " + g.date()}
	return record(id, g.unique("task", 4), h, "## Outcome\n\n"+g.paragraph())
}

func (g *generator) plan(id string, inis, tasks []string) string {
	h := []string{g.maturity(), "part_of: " + g.pick(inis), "derived_from: " + g.pick(tasks), "created: " + g.date()}
	return record(id, g.unique("plan", 4), h, g.paragraph())
}

// steps writes n steps spread over the plans, each following the one before
// in its plan.
func (g *generator) steps(n int, plans []string) {
	prev := map[string]string{}
	for i := 0; i < n; i++ {
		id, plan := g.newID("step"), plans[i%len(plans)]
		h := []string{"part_of: " + plan}
		if prev[plan] != "" {
			h = append(h, "follows: "+prev[plan])
		}
		prev[plan] = id
		g.put("step", id, record(id, g.unique("step", 4), append(h, "created: "+g.date()), g.sentence()))
	}
}

// relyOnBeliefs decides what each commitment relies on. Every fallen belief
// is relied on by a decision or a task, and every revised belief is pinned
// at its first revision by one commitment, so its link is stale. Other
// commitments sometimes rely on a belief at its current revision.
func (g *generator) relyOnBeliefs(all, decs, tasks []*commitment, beliefs []*belief) {
	reliable := append(append([]*commitment{}, decs...), tasks...)
	for _, b := range beliefs {
		switch {
		case b.fallen:
			c := reliable[g.rng.IntN(len(reliable))]
			c.relies = append(c.relies, pinTo(b, b.rev))
		case b.rev == 2:
			c := all[g.rng.IntN(len(all))]
			c.relies = append(c.relies, pinTo(b, 1))
		}
	}
	for _, c := range all {
		if g.rng.IntN(2) == 0 {
			b := beliefs[g.rng.IntN(len(beliefs))]
			if b.fallen || b.rev == 2 || hasLink(c.relies, b.id) {
				continue
			}
			c.relies = append(c.relies, pinTo(b, b.rev))
		}
	}
}

func pinTo(b *belief, rev int) string {
	return fmt.Sprintf("{to: %s, rev: %d, fp: %q}", b.id, rev, b.fp[rev])
}

func hasLink(links []string, id string) bool {
	for _, l := range links {
		if strings.Contains(l, "to: "+id+",") {
			return true
		}
	}
	return false
}

// writeCommitments adds each commitment's relies_on to its file, as the last
// header lines.
func (g *generator) writeCommitments(commits []*commitment) {
	for _, c := range commits {
		if len(c.relies) == 0 {
			continue
		}
		prefix, _, _ := strings.Cut(c.id, "_")
		rel := folders[prefix] + "/" + c.id + ".md"
		var b strings.Builder
		b.WriteString("relies_on:\n")
		for _, l := range dedupe(c.relies) {
			b.WriteString("  - " + l + "\n")
		}
		text := g.files[rel]
		i := strings.Index(text, "created: ")
		g.files[rel] = text[:i] + b.String() + text[i:]
	}
}

func dedupe(links []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range links {
		if to, _, _ := strings.Cut(l, ","); !seen[to] {
			seen[to] = true
			out = append(out, l)
		}
	}
	return out
}
