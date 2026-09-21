# Boundary Substitutes and Controlled Reproductions

Use a substitute only when it preserves the behavior needed to establish the obligation.

## Substitute at real seams

Appropriate seams commonly include:

- external APIs;
- time or randomness;
- operating-system and file-system effects;
- unavailable or unsafe infrastructure;
- a stable interface with multiple real adapters.

Prefer a real lightweight adapter when it is deterministic and practical. Do not mock private methods or internal collaborators merely to assert implementation call order.

## Keep substitutes faithful

A substitute must model the trigger and observable consequence relevant to the check. Keep its behavior narrow and explicit; conditional "mock worlds" that reimplement production policy create a second implementation and a weak oracle.

Accept dependencies at the seam rather than constructing external clients inside the behavior under test. Use operation-specific interfaces so a substitute has one clear result shape instead of a generic dispatcher with test-only conditionals.

## Controlled regression alternatives

When the original failure cannot be reproduced reliably or safely, acceptable alternatives include:

- a faithful isolated reproduction;
- replay of a captured trace whose provenance and relevant fields are known;
- controlled fault injection at the real failure seam.

Record material limitations: what was simulated, which trigger was preserved, which observable failure was checked, and what remains unverified. If those limits leave material uncertainty about protection, seek a human decision rather than report the substitute as complete evidence.
