# Static

Static checks read the source without running it: the build, the type checker, linters, formatters and schema validation.

## Evidence

A clean build and a clean lint prove that the code is well formed and that the properties the tools check hold everywhere they look. They prove no behavior, so they rank below every check that runs the code, and they gate every other check.

## Catalog

An `executed` row per tool or per command that runs several, with the expectation that it exits clean. Listings the repository can print, such as task names or command help, are `generated` rows: they carry no rank and show what the other rows must cover.

## Traps

- A warning that the tool reports and exits zero on is still a finding.
- A tool configured to look at part of the tree proves nothing about the rest; name the scope in the expectations.
