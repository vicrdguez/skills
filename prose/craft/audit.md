### Standards brief

Read the diff and relevant callers. Report every documented-standard violation
and material local-quality concern in changed code or integration effects. Cite
the standard and file/hunk. For a simplification, name the concrete simpler
alternative, the burden it removes, and why required behavior and verification
remain intact. A smell name alone is insufficient. Tag each finding `HARD` or
`JUDGEMENT`; generic smells are always `JUDGEMENT`, while an explicit mandatory
rule or concrete hazard may justify `HARD`. Skip tooling-enforced observations.
Exclude unrelated target additions inherited unchanged. Keep the report under
500 words, compressing rather than omitting findings.

### Contracts brief

Judge the complete final implementation against every accepted behavior,
scenario, Definition of Done, and architecture commitment. Check the current
completion-and-evidence table, not merely the latest delta. Accept grouped
many-to-many evidence, construction freedom, and test reuse or consolidation
when required behavior and failure-mode protection remain. Check removed or
weakened assertions for lost protection without demanding per-test bookkeeping.

Report missing, partial, contradicted, or out-of-scope behavior; frozen
architectural violations; selected-input integrity failures; and specific
coverage/evidence gaps as `HARD`. Quote the obligation and identify the plausible
violation existing evidence cannot distinguish. A red Full Gate is `HARD`.
A plan divergence is `JUDGEMENT` unless it breaks an accepted obligation. Review
integration effects while excluding unrelated inherited target additions. Prior
findings and recorded human directions are evidence within the frozen Contract,
not amendments or new requirements. Keep the report under 500 words, compressing
rather than omitting findings.

