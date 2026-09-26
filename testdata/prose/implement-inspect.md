# Implement inspection continuation

Repository: acme/widgets on remote `origin`
Work Item: widget-dashboard/foundation
Branch: `widget-dashboard`
Worktree: `/work/widgets/.worktrees/widget-dashboard`
Result Documents: `/tmp/skl-implement-result`
Claim: `0000000000000000000000000000000000000001`
Source head: `0000000000000000000000000000000000000002`
Integrated target: `0000000000000000000000000000000000000002`
Review scope: `full`


This read-only continuation resolves the prepared local branch and reports the facts and commands that apply next. It repeats no Contract document, selects no other work, and authorizes no Claim change, ledger mutation, or completion. Re-read the current source facts with `skl implement inspect --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --result-directory '/tmp/skl-implement-result' --target '0000000000000000000000000000000000000002'` after edits and before handoff.

- Continue this Claim with `skl implement resume --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --result-directory '/tmp/skl-implement-result'`.
- Submit settled work with `skl implement submit --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --body '/tmp/skl-implement-result/implement-report.md' --public-body '/tmp/skl-implement-result/public.md' --head <final-source-sha> --target <integrated-target-sha>`.
- Pause on a consequential unresolved decision with `skl implement needs-human --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --body '/tmp/skl-implement-result/implement-report.md' --public-body '/tmp/skl-implement-result/public.md'`.
- Release the reservation without destroying progress with `skl implement release --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001'`.

