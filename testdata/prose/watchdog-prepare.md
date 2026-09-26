# Prepare the reviewed source

Repository: acme/widgets on remote `origin`
Work Item: widget-dashboard/foundation
Branch: `widget-dashboard`
Worktree: `/work/widgets/.worktrees/widget-dashboard`
Result Documents: `/tmp/skl-watchdog-result`
Claim: `0000000000000000000000000000000000000001`
Fixed reviewed implementation head: `0000000000000000000000000000000000000002`
Recorded Integration Target: `0000000000000000000000000000000000000003`
Completed reviews: 0; this invocation is review number 1
Prepared source head: `0000000000000000000000000000000000000002`
Prepared integrated target: `0000000000000000000000000000000000000003`
Source fetch: local: origin fetch failed (exit status 128); using available local source inputs


Source preparation has completed for this Claim. It created or safely reused the exact planned worktree, preserving dirty files, the index, and existing branch progress without resetting, stashing, rebasing, or forcing work aside. Do not repeat preparation; it selected no other work and changed no Workflow State or private ledger record. Read the prepared state with:

`skl watchdog inspect --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --result-directory '/tmp/skl-watchdog-result'`

Continue this Claim with `skl watchdog resume --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --result-directory '/tmp/skl-watchdog-result'`, or release the reservation without destroying progress with `skl watchdog release --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001'`.

