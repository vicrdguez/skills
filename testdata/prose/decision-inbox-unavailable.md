---
name: decision
description: Resolve current Needs Human requests from the ledger-wide Decision Inbox with explicitly scoped human direction.
disable-model-invocation: true
---

# Decision Inbox Unavailable

The configured Workflow Ledger could not be resolved or read, so no inbox was observed. This is not an empty inbox, and no Needs Human request was answered, dismissed, or created.

Reason: unknown Project no-such-project in the configured ledger
Repair: select a Project recorded in the configured ledger, or read the inbox without a filter
Repair the machine configuration or access problem and read the inbox again. The only configured source is `$XDG_CONFIG_HOME/skl/config.json`, falling back to `~/.config/skl/config.json`, with an absolute `ledger` path. The inbox never substitutes a forge search, a source checkout, or agent inference for the missing ledger.
