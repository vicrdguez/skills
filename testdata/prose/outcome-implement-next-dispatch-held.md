Status: stopped

Claim `0000000000000000000000000000000000000001` on Work Item `widget-search/foundation` is still held, so its worker returned before a phase handoff.

Tell the user, and give them both commands to choose from:

- resume the interrupted work: `skl implement resume --repo '/work/widgets' --remote 'origin' --item 'widget-search/foundation' --claim '0000000000000000000000000000000000000001'`
- release the Claim: `skl implement release --repo '/work/widgets' --remote 'origin' --item 'widget-search/foundation' --claim '0000000000000000000000000000000000000001'`

Then stop.
