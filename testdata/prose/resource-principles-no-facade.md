---
statement: Replace an interface outright; no compatibility facade keeps the old one alive beside it.
---

# No compatibility facade

When a change replaces an interface, move every caller to the new one in the same change. A facade that keeps the old interface working doubles what readers must understand and never gets removed.
