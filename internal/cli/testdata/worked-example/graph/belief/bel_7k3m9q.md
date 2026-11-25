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
