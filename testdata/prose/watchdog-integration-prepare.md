# Prepare the reviewed source

Repository: acme/widgets on remote `origin`
Work Item: widget-alerts/foundation
Branch: `widget-alerts`
Worktree: `/work/widgets/.worktrees/widget-alerts`
Result Documents: `/tmp/skl-watchdog-result`
Claim: `0000000000000000000000000000000000000001`
Reviewed head: `0000000000000000000000000000000000000002`
Recorded Integration Target: `0000000000000000000000000000000000000003`
Review round: 1
Prepared source head: `0000000000000000000000000000000000000004`
Prepared integrated target: `0000000000000000000000000000000000000005`
Merged `proposal/widget-alerts` head: `0000000000000000000000000000000000000005`
Source fetch: fresh: fetched origin main and proposal/widget-alerts; widget-alerts is not available from the remote: exit status 128


Preparation is done. Read the prepared state with:

`skl watchdog inspect --repo '/work/widgets' --remote 'origin' --slice 'widget-alerts/foundation' --claim '0000000000000000000000000000000000000001' --result-directory '/tmp/skl-watchdog-result'`

Continue this Claim with `skl watchdog resume --repo '/work/widgets' --remote 'origin' --slice 'widget-alerts/foundation' --claim '0000000000000000000000000000000000000001' --result-directory '/tmp/skl-watchdog-result'`, or release it with `skl watchdog release --repo '/work/widgets' --remote 'origin' --slice 'widget-alerts/foundation' --claim '0000000000000000000000000000000000000001'`.

## Project principles

This repository's principles apply to this work. Read one in full with `skl skill --resource <name>.md principles`.

- `no-facade`: Replace an interface outright; no compatibility facade keeps the old one alive beside it.
- `one-representation`: One native representation; no duplicate payload graph, attribute map or replay log beside it.
