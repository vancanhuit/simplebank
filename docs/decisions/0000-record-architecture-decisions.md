# ADR-0000: Record architecture decisions

## Status
Accepted

## Date
2026-07-15

## Context
SimpleBank is a cloud-native Go service with several load-bearing design
choices (transaction boundaries, the store seam, auth handling, deployment
lifecycle). Reviews on this codebase have repeatedly re-surfaced the same
questions — for example, whether the `Store` interface should be segmented.
Without a written record, each review re-litigates decisions that were already
made deliberately.

## Decision
Record significant architectural decisions as numbered Markdown files in
`docs/decisions/`, using the template below. Write an ADR when a decision is
expensive to reverse, changes a public interface or data model, affects
security posture, or is likely to be questioned again later.

Template:

```markdown
# ADR-NNNN: Title

## Status
Proposed | Accepted | Superseded by ADR-XXXX | Deprecated

## Date
YYYY-MM-DD

## Context
The forces at play: requirements, constraints, and the problem being solved.

## Decision
What we decided to do.

## Alternatives Considered
Each rejected option, with why it was rejected.

## Consequences
What becomes easier, what becomes harder, and any follow-up obligations.
```

## Consequences
- Future contributors and agents can read *why* a decision was made instead of
  re-deriving it or re-opening settled debates.
- Architecture reviews can cite an ADR to close a recurring suggestion.
- ADRs are append-only: a changed decision is a new ADR that supersedes the old.
