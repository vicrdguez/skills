# Contract Acceptance and Finding Criteria

Use these criteria for both implementation Audit and independent Watchdog Review. Sharing them does not invoke Audit again and does not replace either stage's fixed-head, fresh-context, artifact-integrity, or handoff responsibilities.

## Judge three concerns distinctly

Judge behavioral conformance, architectural conformance, and local implementation quality separately.

- **Behavioral conformance** — the delivered behavior and failure modes satisfy every accepted rule and scenario.
- **Architectural conformance** — the implementation honors accepted module, interface, seam, ownership, and other plan commitments.
- **Local implementation quality** — changed code follows mandatory standards and avoids concrete maintainability, security, accessibility, reliability, and compatibility harm.

A green suite is relevant evidence, not proof of all three concerns.

## Account for obligations with credible evidence

Every accepted obligation must be accounted for through grouped many-to-many references to concrete tests, commands, or appropriate inspection evidence. Several obligations may share evidence, and one obligation may require several observations. Ordinary executable behavior needs executable evidence; prose assurance alone is insufficient.

For each claimed check, ask whether it observes the promised consequence and would distinguish a plausible violation. Expected outcomes must be independent of the implementation. Additional executable challenges are warranted by concrete risk or uncertainty, not by a universal demand for another test layer, a one-scenario/one-test mapping, or a duplicate suite.

Assess changed tests together. Reuse, strengthening, consolidation, or removal is acceptable only while required behavioral and failure-mode protection remains covered. Scrutinize removed or weakened assertions for lost protection. Do not require a per-test ledger, a unique-bug quota, or a universal mutation score.

## Classify findings by consequence

A concrete contractual violation, material risk, or specific evidence gap can block even when all existing checks pass. An evidence-gap finding names the obligation, the plausible violation, and why existing evidence does not distinguish it. Merely wanting a different test organization, abstraction, or implementation is not an evidence gap.

An equally valid implementation that satisfies the frozen behavior, architecture, and mandatory standards is not a finding. A reviewer's preference alone neither blocks nor needs a Debt Marker. A concrete nonblocking shortcoming may be recorded as judgement or debt when it states the actual maintenance or product consequence.

During Audit, tag contractual violations, material risks, and specific evidence gaps as `HARD`; tag concrete nonblocking quality debt as `JUDGEMENT`. During Watchdog Review, map active blocking defects to `BLOCK`, unresolved consequential decisions to `HUMAN`, and safe actionable debt to `NOTE` under Watchdog's own disposition and authorization rules.
