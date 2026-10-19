# Maintain the verification skill

Check `.skl/verify.md` against the repository it describes. Read it with `skl skill verify`.

## 1. Run every row

- Each `executed` row: run its command and compare the result with its expectations.
- Each `journey` row: perform its setup and check its sanity predicate.
- Each `generated` row: run it, and compare what it lists with the rows that depend on it: an entry no row covers, or a row naming an entry the listing lacks.
- Each `human` row: confirm what it points a person at still exists.
- Every path and command a row or a section names: confirm it exists.

Check: every catalog row has a result.

## 2. Report

Report each row as one of:

- `clean`: it ran and met its expectations;
- `changed`: the repository moved under it, such as a missing command, task or path, or a listing that no longer matches; name what changed;
- `blocked`: it could not run; name what stopped it.

Check: every catalog row appears in the report with one of the three.

## 3. Update the skill

Edit the file for each `changed` row, keeping the shape in `skl skill --resource verify.md verify-create`, then run each edited row again.

Check: every edited row reports `clean`.
{{template "project-principles" -}}
