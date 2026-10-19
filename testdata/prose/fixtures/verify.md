---
ranks: [live, tests, build]
---

# Widgets verification

## Launch

Run `make serve` from the repository root; the dashboard answers on `http://localhost:8080`.

## Doctor

`make doctor` prints `ready` when Go and the widget database are available.

## Channels

- `driven-surface`: the dashboard page, driven in a browser.
- `tests`: the Go package tests.
- `static`: the Go build.

## Catalog

| Name | Channel | Kind | Run | Expectations | Rank |
| --- | --- | --- | --- | --- | --- |
| dashboard | driven-surface | journey | `.skl/journeys/dashboard.md` | the page shows what the cited rule expects | live |
| widget-tests | tests | executed | `go test ./...` | every test passes, the cited test among them | tests |
| build | static | executed | `go build ./...` | exits 0 | build |
| routes | driven-surface | generated | `make routes` | lists every page the dashboard serves | |

## Cleanup

`make stop` stops the server and drops the scratch database.

## Extend

Add a Go test beside the package a rule changes; add a journey for a page.

## Traps

The server caches widget health for a minute; restart it between journeys.
