# Package tests

Package tests call the code inside the build: unit tests and integration tests that the language's test runner discovers and runs.

## Evidence

A passing test proves its assertion for the cases it sets up, at the seam it calls. Its strength depends on the seam and on the independence of the expected value: a test at a public interface with an expectation taken from the rule is strong; a test that mocks the collaborator under question proves little.

## Catalog

An `executed` row for the whole suite, or one per package or tag when they run separately, with the command and the expectation that every test passes. A Contract cites the row with the test name it adds, so the case identifier finds the test.

## Traps

- A filtered run that matches no test passes; confirm the cited test ran.
- Cached results report a pass without running; run uncached when the result is evidence.
- A skipped or disabled test still prints a green suite.
