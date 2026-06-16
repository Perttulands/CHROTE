# ADR-0004: Formations Judge Verdict Contract

## Status
Accepted

## Context
Formations gates can route on a judge formation's verdict, and formations can run
inline verifications. Both produce a verdict the run engine uses to route work
(pass continues down the pass edge, fail down the fail edge or pushback).

The judge's verdict is read from agent display output text, which is open-ended
prose. The original behavior was effectively fail-open: a verdict that was not a
clean `pass`/`fail` token was coerced toward passing, so the run continued down a
branch the judge never actually authorized. That is dangerous for a gate whose
whole job is to stop bad work: an ambiguous or malformed judge answer silently
became approval.

The main alternatives were:
- keep fail-open and best-effort interpret the verdict text (treat "looks good"
  as pass);
- pick a default branch (always pass, or always fail) on ambiguity;
- require an exact `pass`/`fail` token and block loudly on anything else.

## Decision
Gate judge verdicts and inline verification verdicts use a strict, fail-closed
contract.

A verdict must be exactly the token `pass` or exactly the token `fail`. A judge
formation's verdict is read from its display output text and must BE exactly that
token; the engine does not fuzzy-match, infer from prose, or default a branch.

Any ambiguous or unrecognized verdict blocks the run loudly and routes neither
pass nor fail. A bad gate verdict records a `run_blocked` with code
`ambiguous_gate_verdict`; a bad inline verification verdict records a
`run_blocked` with code `ambiguous_verification_verdict`. Both blocks are
non-resumable: resume must not reinterpret the offending text. The judge, gate
evaluator, or verification must be fixed and a new run started.

## Consequences
A gate can no longer be silently passed by a malformed or hedging judge answer.
Routing only ever happens on an explicit, authorized verdict, so the durable
ledger's pass/fail edges mean exactly what they say.

The trade-off is that judge formations and verifications must emit a clean
`pass`/`fail` token, and an ambiguous answer halts the run instead of guessing.
That is the intended cost: a gate that cannot tell whether it passed must stop,
not assume success. The block is non-resumable, so recovery is an authoring fix
plus a fresh run rather than reinterpreting old output.
