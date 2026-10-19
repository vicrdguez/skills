# Lint Contracts at acceptance with Delegated and Forbidden decisions

A Contract names what to build and leaves the rest to the implementer's reading of the nearest example; acceptance validates only file names. Give the plan two explicit decision classes beside the settled commitments, Delegated and Forbidden, and lint every Contract at acceptance and amendment: labels unique and in their own document, Definition of Done citations resolving, every rule and commitment covered by a Verification strategy row, every row and every Watchdog verdict carrying an evidence rank from an ordered ladder, and every architectural commitment naming its source. A Forbidden violation is a `HARD` Audit finding and a `BLOCK` Review Finding whatever its merit. This refines ADR 0005: headings stay readable and the labels it already mandates become the grammar the engine reads.

## Status

Accepted after the proposal checkpoint of the design-workflow Proposal; implementation is pending.

## Consequences

- The plan document gains `## Delegated` and `## Forbidden`. Delegated names choices that are explicitly the implementer's. Forbidden names decision classes the implementer may not make even though the design never addressed them, such as a new public seam, a compatibility facade, a second representation or paths outside scope; a project principle may be cited there. Anything in neither class is made, recorded in the report as an unplanned decision and reviewed. The engine never reads the text of either section beyond a cited principle name.
- The Verification strategy is a table: covered labels, the check, and its rank. Coverage is many-to-many as ADR 0005 states; the lint requires only that every `B<n>` and `A<n>` appears in some row. The plan is therefore a required Contract document; the lightest holds only the table and the two decision sections.
- An evidence rank is a name from an ordered ladder, strongest first. Without a project ladder the built-in one is `tests`, `build`, `inspection`. A Watchdog `pass` records its rank beside the reviewed head; a verdict is evidence about that head only.
- Every `A<n>` carries a `Source:` line naming an ADR, an experiment report, a checkpoint node or the Proposal's own description. A commitment without a source is an invented obligation and is refused.
- Lint runs while the intake is loaded, before the ledger, the forge or the source tree is touched, and refuses naming the slice, the document and the label. Recorded Contracts are never re-linted; the freeze of ADR 0014 holds, and a later Amendment is linted as any intake.
- No numbering requirement, heading level or ID scheme is added beyond the label grammar; no Markdown dependency is added; the engine judges no finding's merit.

## Evidence

Minos `native-engine/check-ir-structure` implemented IR construction from the JSON schema and kept the trees as the IR against an existing ADR. Audit, Watchdog and merge did not catch it; the human did afterwards, and the slice was superseded. The ADR existed; the slice had no line saying "not that".
