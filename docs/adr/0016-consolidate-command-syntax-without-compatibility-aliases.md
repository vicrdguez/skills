# Consolidate command syntax without compatibility aliases

The CLI consolidation gives each operation one canonical command and flag spelling, removing replaced spellings without compatibility aliases while preserving Workflow Engine behavior. JSON output keys, decision batch-file fields and stored ledger formats retain their existing contracts; generated command strings adopt the new syntax, and historical records remain unchanged. The operator coordinates the binary and installed adapter upgrade between runs, finishing or stopping active runs first; existing Claims remain recoverable through the new resume syntax, with documented recovery rather than command translation or session-migration machinery. This deliberate command-syntax break removes the ongoing burden of duplicate interfaces without expanding into a data migration.

Runtime launch-setting semantics, removal of the launch-resolved marker and native Auto Mode entry points belong to the separate harness-launch-settings proposal. The consolidation builds on that work and preserves its agreed behavior.

Existing parser-error behavior, structured Workflow refusals, status values and exit-code semantics remain unchanged. Help and command-specific validation adopt the canonical interface, including rejection of removed or inapplicable syntax; error-contract standardization is outside this consolidation.

Agent-visible prose changes are surgical. When a file needs only command-syntax updates, those are its only changes; preserve surrounding wording, judgment guidance and step order, and regenerate affected goldens without incidental rephrasing.
