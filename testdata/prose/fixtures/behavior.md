# Dashboard foundation behavior

## B1: The dashboard lists every widget

The dashboard lists every registered widget with its current health.

### Scenario: An unhealthy widget is visible
- Given a registered widget whose last check failed
- When the operator opens the dashboard
- Then the widget is listed as unhealthy
