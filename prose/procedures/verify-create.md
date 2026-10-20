# Create the verification skill

Create `.skl/verify.md` for the repository you are in. Learn it from the repository itself: its files, its tools and the commands you run. Ask the user only what the repository cannot answer. The file's shape is `skl skill --resource verify.md verify-create`.

## 1. Survey the repository

Read the README, the build and task definitions, the CI configuration, the test layout and the entry points. Detect the runtimes, package managers, task runners and test tools from their files.

Check: you can name the command that builds, the command that runs the tests, and every other check CI runs.

## 2. Confirm the channels

Retrieve the craft of each channel and decide whether the repository has it:

- `oracle`: `skl skill --resource oracle.md verify`
- `process`: `skl skill --resource process.md verify`
- `introspection`: `skl skill --resource introspection.md verify`
- `driven-surface`: `skl skill --resource driven-surface.md verify`
- `tests`: `skl skill --resource tests.md verify`
- `static`: `skl skill --resource static.md verify`
- `human-observed`: `skl skill --resource human-observed.md verify`

Note a way of observing the system outside these seven as its own Channels item, with what it observes, and catalog its checks as the nearest channel's craft suggests.

Check: each of the seven channels is present or absent, with the file or command that shows it.

## 3. Build the catalog

Write one row per check the repository can run, as its channel's craft describes, with a concrete command for every `Run`. Order the ranks from the strongest evidence these channels yield to the weakest. For a repository with no checks yet, seed a `build` row and a `tests` row from the language's standard tools, ranked `tests` above `build`.

Check: every command and path the catalog names exists in the repository.

## 4. Write the journeys

Write a journey file for each `journey` row, as `skl skill --resource journey.md verify-create` describes.

Check: each journey row's `Run` names its file, and each file has its preconditions, steps, sanity predicate, cleanup and traps.

## 5. Write the skill

Compose `.skl/verify.md` from its shape and the channel craft, with this repository's commands and paths in every section.

Check: `skl skill verify` renders the file.

## 6. Prove one row

From a fresh shell, following only what the file says, run one `executed` row end to end, or perform one journey through its sanity predicate. Where the file fell short, correct it and run again.

Check: the row passed with the file as your only guide. Report the row, its command and its result, then hand over.
{{template "project-principles" -}}
