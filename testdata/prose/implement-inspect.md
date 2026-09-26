# Implement source inspection

Repository: acme/widgets on remote `origin`
Work Item: widget-dashboard/foundation
Branch: `widget-dashboard`
Worktree: `/work/widgets/.worktrees/widget-dashboard`
Result Documents: `/tmp/skl-implement-result`
Claim: `0000000000000000000000000000000000000001`
Source head: `0000000000000000000000000000000000000002`
Integrated target: `0000000000000000000000000000000000000002`
Review scope: `full`

These are the prepared source facts. Re-read them with `skl implement inspect --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --result-directory '/tmp/skl-implement-result' --target '0000000000000000000000000000000000000002'` after edits and before you submit.

- Continue this Claim with `skl implement resume --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --result-directory '/tmp/skl-implement-result'`.
- Submit settled work with `skl implement submit --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --body '/tmp/skl-implement-result/implement-report.md' --public-body '/tmp/skl-implement-result/public.md' --head <final-source-sha> --target <observed-target-sha>`.
- Pause for a human decision with `skl implement needs-human --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --body '/tmp/skl-implement-result/implement-report.md' --public-body '/tmp/skl-implement-result/public.md' --head <branch-head> --target <observed-target-sha>`.
- Release the Claim with `skl implement release --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001'`.

