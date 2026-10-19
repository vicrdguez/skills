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
- [ ] The `health-corpus` catalog row holds the unhealthy-widget case (B1).

## Manual verification

- [ ] M1: Open the dashboard and confirm each widget's health by hand.

```

## projects/widgets/proposals/widget-dashboard/foundation/plan.md at 0000000000000000000000000000000000000001

```
# Dashboard foundation plan

## Delegated

- The dashboard's layout and styling.

## Forbidden

- A second source of widget health beside the recorded checks.

## Verification strategy

| Covers | Check | Rank |
| --- | --- | --- |
| B1 | widget-tests TestUnhealthyWidgetListed | tests |
| B1 | dashboard | live |
| B1 | new health-corpus unhealthy-widget | tests |

```
