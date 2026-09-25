Status: shown
Project: widgets (acme/widgets)
Work Item: widget-dashboard/foundation
State: ready_for_implementation
Title: Dashboard foundation
Planned branch: widget-dashboard
Dependencies: none
Issue: acme/widgets#101
Reports:
- implement: absent
- watchdog: absent
To inspect earlier rounds, follow ledger input references in the report's original frontmatter and retrieve each exact document with skl ledger show --commit <ledger-commit> --path <ledger-path>. Source references identify source revisions, not ledger documents.

## projects/widgets/proposals/widget-dashboard/foundation/behavior.md at 0000000000000000000000000000000000000001

```
# Dashboard foundation behavior

## B1: The dashboard lists every widget

The dashboard lists every registered widget with its current health.

### Scenario: An unhealthy widget is visible
- Given a registered widget whose last check failed
- When the operator opens the dashboard
- Then the widget is listed as unhealthy

```

## projects/widgets/proposals/widget-dashboard/foundation/intent.md at 0000000000000000000000000000000000000001

```
# Dashboard foundation

## Why

Operators check each widget's health one by one.

## What

Show every widget's health on one dashboard page.

## Definition of Done

- [ ] The dashboard lists every widget with its health (B1).

## Manual verification

- [ ] M1: Open the dashboard and confirm each widget's health by hand.

```
