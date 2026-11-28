# Context graph design, part 3: the implementation

## Why this exists

Part 1 defined the model: small, typed, linked records that separate what we believe and why from what we chose to do, and that know what to reopen when a belief turns out wrong. Part 2 compared it with how others solve the same problems. This part turns the model into something a tool can store, check and query.

Its value:

- **Records stay plain files.** They can be read in an editor, reviewed in a pull request and merged like code. The tool can be thrown away and the knowledge stays.
- **Mistakes are caught when they're written.** A misspelt field or edge is an error with a suggested fix, so a dependency can't silently go missing.
- **Staleness is a query, not a chore.** Every link remembers what it was built against. Asking "what needs a re-check?" is one lookup, and the answer is the same every time the index is rebuilt.
- **Drafting stays cheap.** A record can change freely until something is built on it. Only then do its changes start to matter.
- **There's a path from today's folders.** Each kind of record in today's initiative folders has a place to go, and initiatives can move one at a time.

**How to read this.** Section 1 shows the layout of a workspace. Sections 2–3 cover what a record file looks like and how its fields are checked. Sections 4–5 cover links, pins, revisions and fingerprints, which are the heart of this part. Section 6 is the index and its queries, and section 7 the generated pages. Section 8 covers moving today's initiatives over, and section 9 traces one change end to end. Section 10 lists what can safely come later. Commands are written as `mason <command>`.

## 1. The layout

```
workspace/
├── .loremason/
│   ├── config.yaml        # which loremason version this workspace uses
│   ├── .gitignore         # ignores index.db
│   └── index.db           # the SQLite index, rebuilt from graph/
├── graph/
│   ├── belief/
│   │   └── bel_7k3m9q.md
│   ├── evidence/
│   │   └── ev_p4n2xa.md
│   ├── design-doc/
│   │   └── dd_c5r1tw.md
│   └── …                  # one folder per record type
└── initiatives/
    └── search-redesign/
        └── README.md      # generated, never edited by hand
```

- **`.loremason/` marks the workspace.** Tools find the workspace by walking up from the current folder until they see it, the way git finds `.git`. An environment variable can point at a workspace explicitly. The same folder holds the pinned version and the index, so one marker does three jobs.
- **`graph/` holds every record**, one file each, in one folder per record type. The folder name is the type in kebab-case: `design_doc` records live in `design-doc/`. The graph is shared by the whole workspace. Records aren't filed under an initiative, because knowledge outlives the effort that formed it (part 1, section 3.4).
- **Filenames are the ID only.** A filename built from the title would go stale whenever a revision changed the wording.
- **`initiatives/` holds generated pages**, so browsing an initiative on GitHub still starts at `initiatives/<name>/README.md` (section 7).
- **`index.db` is never committed.** The files are the truth, and any tool can rebuild the index from them.

### IDs

An ID is a short type prefix, an underscore, and six random characters. The characters come from a 32-letter alphabet of digits and lowercase letters, without `i`, `l`, `o` and `u` so that IDs can't be misread. That gives about a billion IDs per type, generated without coordination.

| Type | Prefix | Type | Prefix |
|---|---|---|---|
| `actor` | `who_` | `initiative` | `ini_` |
| `artifact` | `art_` | `prd` | `prd_` |
| `activity` | `actv_` | `requirement` | `req_` |
| `evidence` | `ev_` | `decision` | `dec_` |
| `belief` | `bel_` | `design_doc` | `dd_` |
| `finding` | `fnd_` | `task` | `task_` |
| `question` | `q_` | `plan` / `step` | `plan_` / `step_` |

Sub-records add a slash and a local part: `bel_7k3m9q/s1` is a support link and `q_3fz8km/o1` an option. A revision adds `@`, as in `plan_k2x8wd@r2`.

Steps are records of their own rather than sub-records of their plan. That's what lets a step keep its ID across plan revisions and be re-pinned or revised on its own (part 1, section 5).

## 2. Anatomy of a record

Every record is a markdown file that opens with a YAML header (between `---` lines). **The header holds everything a tool reads; the body holds what a person reads.** Tools never parse the body. They only fingerprint it (section 5). When a tool updates a header, it rewrites only the fields it owns (link lists and `revisions`) and leaves every other line exactly as written, comments and spacing included.

A belief:

```markdown
---
id: bel_7k3m9q
title: validate doesn't handle non-UTF-8 input
claim: validate doesn't handle non-UTF-8 input
status: held
asserted_by: agent
support:
  - {id: s1, evidence: ev_p4n2xa, direction: supports}
  - {id: s2, evidence: ev_h8c1rt, direction: supports}
created: 2026-11-22
---

Both failures are on files that aren't UTF-8 (one Latin-1, one
UTF-16), and both fail at the same read call.
```

A task, with a link pinned to that belief:

```markdown
---
id: task_9w4hzd
title: Report unreadable files clearly
status: accepted
part_of: ini_2m8vqe
implements:
  - {to: req_5txk3a, rev: 1, fp: "fp1:7d0e2a91c4b3"}
derived_from:
  - {to: dd_c5r1tw, rev: 2, fp: "fp1:e81f5530a2d9"}
relies_on:
  - {to: bel_7k3m9q, rev: 1, fp: "fp1:3a9c0e1b77d2"}
created: 2026-11-22
---

## Outcome
validate names any file it can't read and the encoding it expected,
then carries on with the rest.

## Approach
…
```

Fields every record has:

| Field | Meaning |
|---|---|
| `id` | Must match the filename. The type comes from the prefix, so there's no `type` field. |
| `title` | One line a person sees wherever the ID appears. |
| `created` | Date written. When it last changed comes from git. |
| `aliases` | Optional. Other names that resolve to this record, such as an ID from before migration (section 8). |
| `revisions` | On revisable types only (section 5). |
| `extra` | Optional. A free-form area for anything the schema doesn't know yet (section 3). |

Type-specific fields follow the model in part 1. Edges are fields named after the edge, such as `relies_on` or `part_of`, on the record that makes the claim. Sub-records, such as a belief's `support` list or a question's `options`, are lists inside their parent's header, each entry with its own short `id`.

Some types put their main content in the body:

| Type | What the body holds |
|---|---|
| `evidence` | The verbatim quote or observation. Never edited. |
| `finding`, `design_doc`, `prd` | The document itself. |
| `task`, `plan`, `step` | Outcome, approach, the exact instructions. |
| `belief`, `decision`, `question` | Optional reasoning. The claim, choice and wording are header fields. |
| `initiative` | Purpose and goals. |

## 3. Schemas

Each record type has a schema, written in JSON Schema (a widely used standard for describing the shape of JSON and YAML data). Alongside the schemas, a small **type registry** (`types.yaml`) records what JSON Schema can't express:

```yaml
belief:
  prefix: bel
  layer: knowing
  revisable: true
  revisions_inline: true      # keep old wording in the file (section 5)
  meaning: [claim]            # fields that are fingerprinted
  identity: [claim]           # fields that suggest a duplicate
  edges:
    answers: {to: [question], propagates: false}
task:
  prefix: task
  layer: committing
  revisable: true
  revisions_inline: false
  meaning: [title, body]
  edges:
    part_of:      {to: [initiative], propagates: false}
    implements:   {to: [requirement], propagates: true, pinned: true}
    derived_from: {to: [design_doc], propagates: true, pinned: true}
    relies_on:    {to: [belief], propagates: true, pinned: true}
```

- **Meaning fields** are the ones a fingerprint covers (section 5).
- **`document: true`** marks a type as a document: prose we wrote, quotable as evidence. It's set on `finding`, `design_doc`, `prd` and `plan`. A document type must have the shape: revisable (`revisions_inline: false`, so old text stays in git), with `title` and `body` among its meaning fields, `generated_by` (to `activity`) and `attributed_to` (to `actor`) edges, and both as fields in its schema. The registry checks this when it loads, so the next document type needs one line, and a type with the wrong shape is refused. It's the same idea as `commitment`: a shared shape, never stored as a type. An edge's `to` may name `document` beside types and `any`, meaning every document type. Evidence's `from` is `{to: [actor, artifact, document], propagates: false, pinned: true}`.
- **Identity fields** let `validate` warn that a new record looks like an existing one and suggest using that instead. This mainly helps sources: two `artifact` records with the same URL are probably the same thing, and two `activity` records with the same `session` (section 6) are the same session.
- **`propagates`** says whether a change in the target means this record needs a re-check. Part 1, section 3.2 lists those edges. Writing it down per edge type is how the "per-edge propagation" in part 1, section 8 starts.
- **Statuses with a meaning** are data too, each list checked against the type's schema:
  - `fallen` lists a type's statuses that mean the record no longer holds;
  - `closed` lists the statuses that mean a record is no longer in force, for any type (for a belief, `withdrawn`; section 6);
  - `sealed` lists the statuses at which the record's current revision is sealed (section 5): for a decision, `accepted`, `rejected`, `deprecated` and `superseded`. A question has no status, so for it the key is "settled"; for a step, which has no status of its own, it is its plan's status. `status_from` names the edge a status-less record takes its status from (a step's `part_of`); the record's in-force state and `sealed` key follow it;
  - on an edge, `target_status` lists the statuses the target should be in (`supersedes` to `superseded`). It applies only when the source is in force and sealed by its status (for a decision, `accepted`), so a draft that isn't yet in force isn't warned about;
  - on an `answers` edge, `settles` lists the source's statuses in which it settles the question (a decision at `accepted`, a belief at `held`), and `pending` lists those in which it is an answer still waiting (a decision at `proposed` or `leaning`).

**Where work stands is separate from how settled it is.** A task's `status` says how settled the commitment is. Its `progress` says where the work is: `todo` (the default, so a task without the field is `todo`), `in_progress`, `blocked` or `done`. A blocked task can name what it waits on in `blocked_by`, a list of links to any record. A question has no status: it is open until an answer settles it, or until it is marked `dropped:` with the reason. Answering a question never edits it. Neither progress nor dropping is part of a record's meaning, so neither makes a link stale.

**Strict fields, plus a free-form area.** A field the schema doesn't list is an error, and `validate` suggests the closest known one ("unknown field `relies_om`; did you mean `relies_on`?"). Edge names follow the same rule. Anything can go under `extra:` without being checked:

```yaml
extra:
  confidence: 0.7               # an experiment; not part of the schema yet
  inspired_by: [fnd_a1b2c3]     # an experimental edge: indexed, never propagates
```

A field that proves useful is promoted into the schema later. That's an addition, so records already written stay valid. Edges under `extra:` show up in queries but never carry staleness or invalidation, which keeps propagation predictable.

Why strict: a warning lets a typo such as `relies_om` silently drop a dependency, and then invalidation never reaches that record. Warnings that are usually harmless also teach people to ignore them. The free-form area keeps experiments possible without forking the schemas. Kubernetes works the same way, with strict fields beside free-form labels and annotations.

## 4. Links and pins

A link appears in one of two forms:

| Form | Written as | Used for |
|---|---|---|
| **Bare** | `relies_on: [bel_7k3m9q]` | What a person or agent writes. Also every link that doesn't propagate (`part_of` an initiative, `mentions`, `answers`). |
| **Pinned** | `{to: bel_7k3m9q, rev: 1, fp: "fp1:3a9c0e1b77d2"}` | What a link that propagates becomes, and evidence `from` a document. It records the target's revision and fingerprint at the moment the link was made or last checked. |

**People never type fingerprints.** They write bare links, and `mason sync` pins each new link that propagates, and each new evidence `from` a document, to its target's current state. A bare link that propagates is valid but not yet watched, so it can never be stale. `mason sync` runs as part of the skills that write records, so in practice links are pinned within the same change.

A pinned link can carry one more field: `via: ev_…`. It names the evidence recording the last re-check that concluded "still holds" (section 5), so the reason for a re-pin is one click away.

Some edges never propagate. `part_of` to an initiative, `involves`, `answers` and `mentions` are always bare. Evidence `from` a document (a finding, design doc, PRD or plan) is pinned, so the quote can be traced to the revision it was taken from, but it doesn't propagate and it doesn't seal that revision (section 5): revising a document doesn't make its old quotes wrong.

## 5. Revisions, fingerprints and re-checks

### Revisions in the file

A revisable record (beliefs, questions, findings, steps and every commitment) keeps a `revisions` list in its header. The last entry is the current revision:

```yaml
revisions:
  - rev: 1
    date: 2026-11-22
    fp: "fp1:3a9c0e1b77d2"
    claim: validate doesn't handle non-UTF-8 input    # old wording, kept inline
  - rev: 2
    date: 2026-11-29
    fp: "fp1:91bd04e6c2a8"
    change: breaking
```

- **`change`** says whether the revision is `breaking` or `non_breaking` compared with the one before. If it's left out, the revision counts as breaking. Removing or narrowing something is breaking; adding something isn't.
- **Old text is kept inline for short records** (beliefs, questions, requirements, steps), so a reference to `@r1` can be read without git.
- **For long documents, the old text stays in git.** That covers findings, design docs, PRDs, tasks, plans and decisions. Their entries hold a `commit` instead of the old text, so a file doesn't fill up with old drafts. Staleness never needs the old text, only the fingerprints, which are always in the file.
- **A record with no `revisions` list is at revision 1.** That's how a new record starts, so `@r1` can be named straight away. `mason sync` writes the entry, with its date and fingerprint.

### Drafting stays cheap: a revision is sealed when something relies on it

A revision can be edited freely until it is sealed. `mason sync` keeps its recorded fingerprint up to date, and no new revision is made. Two things seal a record's current revision:

| Type | What seals it |
|---|---|
| belief, finding, design doc, PRD, requirement, task, plan | The first propagating pin on it (a link that propagates and names this revision) |
| decision | A propagating pin, or its status reaching `accepted` (and after: `rejected`, `deprecated`, `superseded`) |
| question | A propagating pin, or being settled |
| step | A propagating pin, or its plan's status reaching `accepted` |

A status seals a revision only once `mason sync` has recorded its fingerprint, so a revision `mason revise` has just opened can still be edited until the next sync. Until then `validate` warns that the status will seal it once it is written down.

Which statuses seal is data, the `sealed` key in `types.yaml`. The status rule makes "in force" mean "unchangeable in place": settling a decision is a visible act, and after it a change of meaning is a new revision. Once a revision is sealed, a change of substance needs a new revision. **The revision comes first, then the edit:**

1. `mason revise <id> --breaking` (or `--non-breaking`) keeps the current wording inline, or for long records the commit that holds it, and opens the next revision. It needs one of the two flags, so an agent never meets a prompt.
2. Edit the record.
3. `mason sync` records the new revision's fingerprint, since nothing seals it yet.

Doing it in that order means the old wording is always still in the file when it's kept, with no need for git. If someone changes a sealed record's substance without a revision, `validate` fails and says to undo the change, run `mason revise`, then make it again. `revise` refuses to run in that state, so the changed wording can't be kept by mistake as the old one.

**Quotes pin but don't seal.** The one pinned link that doesn't propagate is evidence `from` a document. Its pin records the revision and fingerprint at the moment of quoting. It doesn't force a revision, but it lets you make one: sealed means you must revise, quoted means you may.

- It doesn't seal the revision, so the document stays free to edit. `mason sync` keeps refreshing the unsealed revision's fingerprint as before.
- It's never stale, and `sync` never moves it forward.
- `validate` warns when the quote's recorded fingerprint no longer matches the quoted revision's text, since the document was edited after the quote was taken. The warning says to run `mason revise <id>` before editing (undo the change first), so the quoted revision is kept. Doing so keeps that revision's wording and fingerprint, the quote keeps naming it, and the warning clears. If the quoted revision is already past, the warning says to restore the text or write new evidence.
- The quote stays traceable. The evidence holds the verbatim text, and the pin's revision plus git history (the document as it was when the evidence was written) show where it came from.

The seal exists so that a change reaches the work built on it. A quote doesn't need that, because it doesn't propagate and already holds its text. Findings keep evolving after someone quotes them, so sealing on a quote would get in the way of ideation.

This is what keeps the structure out of the way while ideas are forming. A design doc, finding or plan can go through many drafts in a day. Revisions only start once other work is built on it through a link that propagates, or its status says it is in force.

Status changes and new support links are never substance. They're edited in place, and git keeps their history. (A status change can still seal, as the table shows.) One flip isn't caught: setting an accepted decision back to `proposed`, editing it, and accepting it again. Git history shows it.

### Fingerprints

A fingerprint is a short code computed from a record's meaning fields. If the fingerprint is unchanged, the meaning is unchanged. Formatting edits leave it alone, so they never raise staleness.

```
fp1:3a9c0e1b77d2
└┬┘ └─────┬────┘
 │        └ first 12 hex characters of a SHA-256 hash of the canonical form
 └ the method's version
```

The **canonical form** is fixed precisely, because any drift would look like every record changing:

1. Take the record's meaning fields from the registry. `body` stands for the body.
2. Write the fields as compact JSON with sorted keys, in UTF-8. Characters are written as themselves, except what JSON itself requires to be escaped (quotes, backslashes, control characters) and the line and paragraph separators U+2028 and U+2029, written as `\u2028` and `\u2029`. `<`, `>` and `&` are written as themselves.
3. Normalise the body: Windows line endings (`\r\n`) become Unix ones, trailing spaces and tabs are removed from each line, runs of blank lines become one, and blank lines at the start and end are dropped. A line holding only spaces counts as blank. A lone `\r` is left as it is.
4. Write IDs inside fields bare, without `@rN`.
5. Hash the result with SHA-256 and keep the first 12 hex characters.

The `fp1:` prefix names the method. If the method changes, say to stop counting markdown formatting as change, it becomes `fp2:`, and tools compare like with like. They never read a method change as every record changing.

### Re-checks

When a pinned link no longer matches its target, the record holding it is stale. An agent re-checks it and ends in one of two ways:

| Outcome | What changes on disk |
|---|---|
| **Still holds** | The pin is updated to the target's current revision and fingerprint, with `via` pointing at a new evidence record that says why (`about` the record, `generated_by` the agent's session). The record itself gets no new revision, so nothing below it is touched. That's early cutoff. |
| **Needs to change** | The record gets a new revision through `mason revise`, and its pins are updated. Whatever pins *it* now becomes stale, one step further down. |

**Non-breaking revisions re-pin quietly.** If every revision between a pin and its target's current one is `non_breaking`, `mason sync` moves the pin forward with no re-check and no evidence. The revision's own `change` value is the record of why, so a `via` from an earlier re-check is kept: it still names the evidence for that re-check.

Pins, `via` links and re-check evidence all live in record files, never only in the index. So a rebuild from the files always gives the same answer to "what is stale?".

## 6. The index

`index.db` is a SQLite database (a database kept in a single file). `mason` is written in Go and includes SQLite through a pure-Go driver, so it stays one binary with nothing else to install. The index is rebuilt from `graph/` whenever the files have changed since the last build: the index keeps a signature of every file's path, size and modification time, so an added, edited or deleted file is noticed. It's also rebuilt when the loremason version or fingerprint method has changed, when the index's own table layout has (a schema version number in the code), or when the record types have (`types.yaml` and the JSON schemas; the index keeps a hash of them). When `.loremason` can't be written to (a read-only checkout), the index is built in a temporary file outside the workspace on every command, and removed when the command ends. Several `mason` commands starting at once on a stale index each build into their own temporary file and rename it into place, so none of them sees half an index and none fails. A full rebuild of 5,000 records took about a third of a second in a test, so there's no incremental update to get wrong.

```mermaid
erDiagram
  nodes ||--o{ edges : "src"
  nodes ||--o{ edges : "dst"
  nodes ||--|| nodes_fts : "searched by"
  nodes ||--o{ aliases : "id"
  nodes ||--o{ members : "belongs to"
  nodes ||--o| initiatives : "is"
  nodes ||--o{ produced : "generated by a session of"
  nodes {
    text id PK
    text type
    text title
    text status
    text progress "computed"
    bool settled "computed"
    int revision
    text fingerprint "current, computed"
    text path
    json header
    text body
  }
  edges {
    text src FK
    text dst FK
    text name "the field the link is written in"
    int pinned_rev
    text pinned_fp
    bool propagates
    int concerns "computed"
    bool settles "computed"
    bool pending "computed"
    text role
  }
  members {
    text initiative FK
    text id FK
  }
  initiatives {
    text id PK
    text slug
    text purpose
    json goals
  }
  produced {
    text initiative FK
    text id FK
  }
  aliases {
    text alias PK
    text id FK
  }
  nodes_fts {
    text id
    text title
    text body
  }
```

- **`nodes`** has one row per record and sub-record. Sub-records such as `bel_7k3m9q/s1` get their own rows so that edges like `undercuts` can point at them.
- **`edges`** has one row per link that reaches a record, including a containment link, named `subrecord_of`, from each sub-record to the record holding it. Containment doesn't propagate, and `mason show` leaves it out: it lists a sub-record's own links with the record's, marked with the sub-record's local name. Whether a link comes from `extra:` (an experimental link) isn't stored yet; no query needs it. Reverse links aren't stored; a query reads `edges` from the other side.
- **`nodes_fts`** is SQLite's built-in full-text search (FTS5). It lets a person or agent find "the belief about non-UTF-8 files" from plain words.
- **`aliases`** maps each alias a record has had to its ID, so `show` and `dependents` accept an old one.
- **`members`** lists the records in each initiative, sub-records included. A record is in an initiative when following `part_of` from it reaches the initiative, so requirements are in it through their PRD and steps through their plan.
- **`produced`** lists the records generated by an activity that is a member of the initiative: the findings and evidence its sessions made. They aren't members, because knowledge doesn't belong to an initiative; this is the trail that answers "what did this initiative learn".
- **`initiatives`** holds each initiative's slug, purpose and goals, for `mason where` and the generated README.
- A **`meta`** table records what the index was built from: the loremason version, the fingerprint method, the index's schema version, a hash of the record types (`types.yaml` and the JSON schemas), and the files' signature.

**The rules are computed once, in Go, and stored.** Whether a link needs a re-check, and why, is one value, its concerns: any of stale (its pin no longer matches the target's current revision and fingerprint), reopens, orphaned and doubted, all worked out in one place by the same code `sync` and `validate` use. They are written to the one column `edges.concerns`. A link has concerns only when it propagates, its source is a commitment still in force, and its target exists. The queries only select on that column, so a rule has one home and `stale` and `where` can never disagree with `sync` or each other.

The same goes for where work stands: a task's progress (with its default), whether a node is settled (dropped, or settled by an incoming `answers` link), whether each `answers` link settles or is pending, and each initiative's members are worked out in Go and stored, and `mason where` only selects.

A link **reopens** its source when it propagates, its source is a commitment still in force, and its target's status is one its type marks as fallen. Which statuses count as fallen is data, declared in `types.yaml` beside each type's statuses: for a belief, `incorrect` and `insufficient`. Which statuses close a record is data too (for commitments, `rejected`, `deprecated` and `superseded`; for a belief, `withdrawn`), so a commitment already turned down or replaced isn't listed again. A belief that is withdrawn reopens nothing, but a commitment in force that relies on one is reported separately ("rests on bel_x, which is withdrawn"). Generally, a propagating link from a record in force to a closed record is reported as needing a re-check, like a stale link ("rests on X, which is <status>"), and it is one of the link's concerns, so `mason stale` and `mason where` list it.

A belief is doubted when evidence undermines evidence one of its `supports` links cites, or undercuts one of those links. The commitments relying on a doubted belief show in `where`'s re-check list. The quiet re-check writes evidence and sets `via` on the `relies_on` pin. Once that evidence is observed on or after every undermining or undercutting evidence (compared as instants of `observed_at`: a date alone is midnight UTC, a time with no offset is UTC, and a tie clears the doubt), the doubt clears; a newer undermine brings it back. Only `supports` links count. Evidence whose `observed_at` is missing or unreadable clears nothing. Until a `mason recheck` command exists, `via` is set by hand.

`validate` warns about records that disagree with each other: a `held` belief with a support link whose direction is `disputes` (it prompts setting the belief to `incorrect`); a decision whose `chooses` names an option of a question it doesn't `answers`; two decisions in force that settle one question; a decision that `supersedes` another that isn't `superseded`; and a task with more than one plan in force. The edge-shaped ones are data in `types.yaml` (`within`, `target_status`, `one_in_force`, documented in its header) and checked generically; the belief ones are written in the check and the index. `target_status` applies only when the source is in force and sealed by its status (for a decision, accepted). A question is open until an answer settles it (an accepted decision, or a held belief); while a decision answering it is `proposed` or `leaning`, it stays listed with the answer waiting, whatever else answers it; a dropped question isn't listed. Evidence's fingerprint is recorded by the first `mason sync` after it is written (not while the body is still the template), and `validate` fails if it changes. A plan is `part_of` its initiative, like every commitment (the plan template has a `part_of` field).

Following links is done in Go, not in SQL. `mason dependents` loads the links that propagate once and walks them breadth-first, remembering each record it has seen, so a record is listed once at its shortest depth and a cycle or a densely linked graph costs one visit per record. (A recursive SQL query, `WITH RECURSIVE`, visits every path, and the number of paths grows explosively.) Walks like this are used to follow and explain, such as listing everything built on a record. They're never used to mark a whole chain stale.

**A graph with errors still answers.** The index holds everything that loads, and each query warns that the graph has errors and that `mason validate` will list them. An agent fixing an error can still query its way around it.

**Named queries** are the everyday interface, the same for people and agents:

| Command | Answers |
|---|---|
| `mason find <words>` | Which records match these words? Takes one or more words, unquoted if you like. Shows each record's ID, type, status and title. |
| `mason show <id>` | One record's text, with its links out and the links pointing at it, each showing the record at the other end by ID, type, status and title. |
| `mason dependents <id>` | What is built on this, directly and further down? Follows links that propagate, with each record's depth; a record that only mentions this one isn't listed. |
| `mason stale` | Which links need a re-check (stale pin, reopened, orphaned or doubted), and why, grouped by the record that changed? `mason reopened` lists only the reopened ones. |
| `mason reopened` | Which commitments rely on a belief that has fallen? |
| `mason where <initiative>` | The "where are we" view of an initiative, named by slug, ID or alias: tasks in progress, blocked tasks with what they wait on, open questions with their pending answers, and the links from the initiative's records (and their sub-records) that need a re-check, grouped by the record that changed. |

Queries print plain text, one stable line per result, which people and agents both read. Structured output can come with the MCP server (section 10).

Writing commands are kept separate and few. `mason new <type>` creates a record with a fresh ID, `mason revise <id>` makes a new revision, and `mason sync` fills pins, refreshes unsealed fingerprints, re-pins across non-breaking revisions and regenerates views. `validate` only reads, and fails on any error.

`mason sync` works in a fixed order, because a propagating pin (or a status that seals) fixes the revision it names: first it writes revision 1 where it's missing, then it records fingerprints of revisions that aren't sealed yet, then it pins bare links, then it re-pins across non-breaking revisions. It leaves a file with errors alone and says so, because re-writing a link list it can't read could lose what a person wrote. `mason sync --dry-run` says what it would do. Rewritten fields always take one form: links as a block list of `{to, rev, fp, via}` maps, revisions as a block list of maps. Every other line keeps its bytes, comments and spacing.

**Sessions attach what they write to an activity.** Knowledge belongs to no initiative (part 1, section 3.4), so a finding's trail to its initiative goes through the activity of the session that wrote it. `mason new` attaches that activity itself for findings and evidence, so nobody has to remember to. The session is named by environment variables:
- `LOREMASON_SESSION` is any key the harness gives the session, such as an agent's session ID set by a session-start hook.
- `LOREMASON_INITIATIVE` names the initiative by slug, ID or alias, as `mason where` accepts.
- `LOREMASON_ACTOR` is optional, and names who the activity is `associated_with`. It must be a reference such as `who_4d9snc`.

A `LOREMASON_ACTOR` that isn't a reference, or a `LOREMASON_SESSION` that spans lines, is a usage error, since either would break the activity's header.

When `mason new` creates a finding or a piece of evidence, it attaches the session's activity, creating it on first use. These are the knowledge with no other path to its initiative. Design docs, PRDs and plans are commitments, already `part_of` the initiative. They have a `generated_by` link that can be filled in by hand, but `mason new` doesn't fill it. A new activity is `part_of` the initiative, has a `started_at`, is `associated_with` the actor when one is given (written as a block list), and holds the key in its `session` field. `started_at` is local time with its offset.
- **One activity per session.** Its ID is derived from the session key, so concurrent commands find the same file and never create two. A command that finds the file already there uses it, and a file whose `session` doesn't match the key is an error. `validate` warns when two activities claim one session, since `session` is the activity's identity field.
- **Without a session, nothing fails.** The record is created unattached and `mason new` prints a one-line note to stderr saying so. `validate` doesn't warn about an unattached document, so a hand-written finding stays free.
- **A session with no initiative** is a usage error when its activity doesn't exist yet, naming `LOREMASON_INITIATIVE`. Once the activity exists, the initiative is read from it, and a `LOREMASON_INITIATIVE` that names another one is ignored, with a note to stderr.
- **Ending a session** (`ended_at`, `status`) is left to the harness for now, for example a session-end hook. Nothing reads them yet.

Nothing is kept outside the records, so parallel sessions in one checkout don't collide.

## 7. Generated pages

Each initiative's `README.md` is built from the records in it and written to `initiatives/<slug>/README.md`, so browsing an initiative on GitHub still starts there. It shows the title, purpose, goals and body of the `initiative` record, who is involved and their roles, and the initiative's design docs, PRDs and tasks. Each is linked by title to its record file and shows its status, and a task its progress. After the tasks comes a Findings section: the findings the initiative's activities produced (section 6), linked by title and showing no status, since findings have none. It's left out when there are none. Closed records stay listed, because the page is the list of what exists.

After its title, the page has a hidden marker line (line 3) and a visible line saying it's generated and from which record. It's a read-only snapshot: nothing reads it back except to compare it with a fresh render. Its rules:
- **Kept current.** `mason sync` regenerates it, and `validate` fails when it no longer matches the records, so a hand edit can't quietly drift.
- **Committed.** It's marked generated in `initiatives/.gitattributes`, where `sync` keeps its one line (`*/README.md linguist-generated=true`) and leaves every other line alone, so GitHub collapses it in pull request diffs. A merge conflict in it is resolved by running `sync`.
- **Stable.** It renders the same bytes from the same records on any day: it holds no dates.
- **Never over a person's work.** `sync` writes only a page with no file yet or with its marker among the first ten lines. A hand-written `README.md` is left alone and reported, and `sync` exits with failure until it is moved. A generated page whose initiative is gone is deleted, unless the records had errors, since a broken record may be the missing initiative. Two initiatives with the same slug get no page.

There is no committed `where.md`. A copy of the `mason where` view would change with nearly every record change, conflict across branches and be read stale, while the command answers fresh. Writing its output to a file can come later, and so can a UI over the same queries.

## 8. Moving today's initiatives

Before loremason, each initiative was a folder holding JSON logs, markdown findings, tasks, plans and a scratchpad. Here's where each part goes:

| Today | Becomes |
|---|---|
| `README.md`, `initiative.json` | An `initiative` record; the README is generated from now on. |
| `stakeholders.json` | `actor` records, with an `involves` link on the initiative carrying each role. |
| `questions.json` | `question` records, `part_of` the initiative. |
| `decisions.json` | `decision` records. Each decision's options move to the question it answers. Its free-text context becomes beliefs only where they're load-bearing; otherwise it stays as the decision's body. |
| `findings/` | `finding` records with their prose unchanged, each `generated_by` an `activity` for the session that wrote it, which is `part_of` the initiative. |
| `tasks/`, `plans/` | `task` and `plan` records. Old plans keep their steps in the body; only new plans get `step` records. |
| `quizzes/` | An `artifact` for the conversation, plus evidence quoting the answers. |
| Scratchpad (`CONTEXT.md`) | Not migrated as a record. Its useful content becomes questions, beliefs or leanings by hand, and `mason where` answers what it used to. |

**Old IDs keep working.** Each migrated record lists its old ID in `aliases` (`search-redesign/F-002`). So old references in prose still resolve through `mason show`, and nothing has to be rewritten at once.

**One initiative at a time.** The old checker keeps validating the initiatives still in the old format, and the new tool validates `graph/`. The two run side by side, and the old checker retires when the last initiative moves. The first initiative to move is a pilot, ideally one doing open-ended design work. Moving it is also the test of whether this structure gets in the way of open-ended design work, or helps it.

The order of building, from an empty loremason repository to a working workspace, is a plan of its own, written after this part is agreed.

## 9. Worked example: a belief changes

The IDs follow part 1, section 5.

**1. Before.** The task above relies on `bel_7k3m9q@r1`. A plan, `plan_k2x8wd`, is `derived_from` the task at revision 1. Its three steps are each `part_of` the plan at revision 1. `mason stale` returns nothing.

**2. The belief is revised.** New evidence shows UTF-16 files are read fine and only Latin-1 fails. The claim narrows to "validate doesn't handle Latin-1 input". Narrowing a claim is breaking, so an agent runs `mason revise bel_7k3m9q --breaking`, which keeps the old wording inline as revision 1 and opens revision 2. Then it edits the claim and runs `mason sync`, which records revision 2's fingerprint.

**3. One link goes stale.**

```
$ mason stale
validate doesn't handle Latin-1 input  (belief, r1 → r2, breaking)
  └ Report unreadable files clearly  (task, accepted)
```

Only the task appears. The plan and steps aren't stale: their pins point at the task, and the task hasn't changed.

**4. The re-check stops early.** An agent re-reads the task against the new claim. Reporting unreadable files clearly is still the right outcome. The agent writes evidence `ev_m3q8vb` ("still holds: the outcome doesn't depend on which encodings fail") and moves the task's pin to `{to: bel_7k3m9q, rev: 2, fp: "fp1:91bd04e6c2a8", via: ev_m3q8vb}`. The task gets no new revision, so the plan is never touched.

**5. Had it gone the other way.** Say the task's approach named UTF-16 detection specifically. The agent revises the task (`task_9w4hzd@r2`, breaking). Now the plan's pin is stale, and the plan is re-checked. If its steps need to change, the plan gets a revision and each step is re-checked in turn. A step that still holds re-pins and stops.

**6. Had the change been cosmetic.** If someone only re-wrapped the belief's reasoning, its meaning fields wouldn't change, so neither would its fingerprint. Nothing would be stale, and no revision would be needed.

At no point did a person see a flag. This example leaves out part 1's decision, which also relies on the belief. Its link would go stale in step 3 as well, and because the decision is `accepted`, its re-check would come to a person as a drafted change, never as a bare warning (part 1, section 4).

This walk-through runs as tests (`internal/pinning/belief_change_test.go`).

## 10. Can safely come later

None of these changes the file format. Each can be added without rewriting records.

- **An MCP (Model Context Protocol) server**, so agents can call the named queries as tools rather than through the command line. Basic Memory is the model to learn from.
- **Hybrid search**, adding vector search (`sqlite-vec`) beside full-text search.
- **Incremental index updates**, if full rebuilds ever get slow.
- **Whether `validate` becomes `mason validate`.**
- **The ideas part 1, section 8 already lists:** pinning a section of a long record and quote selectors, a second fingerprint for metadata, recording who made each link, more run fields, the initiative lifecycle, ownership, and grouping stale records by root cause.
- **A PROV export**, writing the graph out in the W3C provenance standard's own format, for example through Oxigraph.

## Coming in later parts

- **Assembling context per reader:** which records each reader gets at each moment, and which they must not see, built on `mason show`, `mason where` and the two-tier pattern from part 2.
- **The build plan** (not a design part): building the new tool up step by step in the loremason repository, then moving initiatives onto it, starting with a pilot.

## Appendix: Terms new in this part

| Term | Plain meaning |
|---|---|
| **header** | The YAML block at the top of a record file. Everything a tool reads. |
| **type registry** | `types.yaml`: per record type, its ID prefix, meaning and identity fields, and which edges propagate. |
| **meaning fields** | The fields a fingerprint covers. A change elsewhere is formatting or metadata. |
| **identity fields** | Fields that suggest two records are the same thing, such as an artifact's URL. |
| **bare link / pinned link** | A link as written, or with the target's revision and fingerprint recorded. |
| **sealed** | A revision that a link that propagates has pinned, or whose record's status says it is in force (a decision once `accepted`, a question once settled, a step once its plan is `accepted`). A quote's pin (evidence `from` a document) doesn't seal. From then on, a change of substance needs a new revision. |
| **canonical form** | The exact text a fingerprint is computed from, fixed so that the same meaning always gives the same fingerprint. |
| **`via`** | On a pinned link: the evidence recording the re-check that last re-pinned it. |
| **generated page** | A page built from records, committed, and never edited by hand. |
| **alias** | Another name for a record, such as its ID before migration. |

