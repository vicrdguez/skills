---
name: implement
description: Implement a single claimed change against its accepted behavioral and architectural contract.
disable-model-invocation: true
---

# Implement source preparation

Repository: acme/widgets on remote `origin`
Work Item: widget-dashboard/foundation
Branch: `widget-dashboard`
Worktree: `/work/widgets/.worktrees/widget-dashboard`
Result Documents: `/tmp/skl-implement-result`
Claim: `0000000000000000000000000000000000000001`
Source head: `0000000000000000000000000000000000000002`
Integrated target: `0000000000000000000000000000000000000002`
Source fetch: local: origin fetch failed (exit status 128); using available local source inputs


Source preparation has completed for this Claim. It created or safely reused the exact planned worktree, preserving dirty files, the index, and existing branch progress without resetting, stashing, rebasing, or forcing work aside. Do not repeat preparation; it selected no other work and changed no Workflow State or private ledger record. Read the prepared state with:

`skl implement inspect --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --result-directory '/tmp/skl-implement-result' --target '0000000000000000000000000000000000000002'`

Continue this Claim with `skl implement resume --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --result-directory '/tmp/skl-implement-result'`, or release the reservation without destroying progress with `skl implement release --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001'`.

