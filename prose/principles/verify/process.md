# Process

The process channel runs the system as a user does, as a separate process, and observes what comes out: exit status, standard output and error, and the files it writes.

## Evidence

A process check asserts the status and the output of one invocation against prepared inputs. It proves what a caller sees, through the real argument parsing, configuration and I/O, and is stronger than a unit test of the same rule. It stays as strong as its assertions: a check that only asserts a zero exit proves the process ran.

## Catalog

An `executed` row for the suite of process tests, with the command that runs them and the expectation that every test passes. A Contract cites the row with the test it adds. A hand-run invocation becomes a row once it is scripted with its expected status and output.

## Traps

- The process inherits the environment: home directory, configuration, network, clock and locale. Pin each one the output depends on.
- A fixture shared between invocations carries state from one to the next; build fresh fixtures, or reset them, per check.
- Output that differs per run (paths, identifiers, timestamps) is normalized before comparing, and only those values.
