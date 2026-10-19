# Executable oracle

An executable oracle compares the system's output with an expectation it did not produce: a conformance corpus, golden files, a reference implementation, a property or metamorphic relation.

## Evidence

A run reports, per case, whether the output matched. The strength lives in the expectations: an expectation written from the rule, or recorded and reviewed once, can disagree with the code; an expectation regenerated from the current output only records what the code does. A corpus with a shrinking list of unsupported cases also shows coverage moving.

## Catalog

One `executed` row per oracle, run by the command that runs every case and fails on any mismatch. Its expectations name where cases live and what a pass prints. A Contract cites the row with the case it adds, so the case identifier says which expectation proves the rule.

## Traps

- A regenerate or update flag turns the oracle into a recorder: the check that proves a rule compares, and a reviewer reads every regenerated expectation as a change.
- Normalization that hides run-specific values can hide a real difference too; keep it to values that vary between runs.
- An expected-failure or skip list grows quietly; read it on every run.
