# Binary releases

A human merges the intended release changes into `main`, then creates and pushes
an annotated stable tag at the intended commit:

```sh
git switch main
git pull --ff-only origin main
git tag -a v0.5.0 -m 'skl v0.5.0'
git push origin v0.5.0
```

`v0.5.0` is the first planned binary release. Do not create it until these changes
are merged. Merging alone does not release. The tag workflow accepts only stable
`vX.Y.Z` tags (no leading zeros), and checks that the tagged commit is contained
in `main`. Prereleases and other tags do not publish.

The workflow uses Go from `go.mod`, Git, shell and tar, checks out full history
(the prose-metrics tests need it), and runs `go test -timeout 20m ./...`. With CGO disabled it
builds macOS and Linux, each for AMD64 and ARM64. Each archive is named
`skl_<tag>_<darwin|linux>_<amd64|arm64>.tar.gz` and contains the executable `skl`
at its root, including embedded workflow prose. `skl --version` reports the tag.
`checksums.txt` contains SHA-256 hashes of the four archives. Cross-compilation
is build evidence, not proof of execution on every target.

All archives and checksums are prepared before creating or uploading to a draft
GitHub Release. GitHub-generated notes are created with the draft. The workflow
publishes automatically as a normal, non-draft release only after every upload
succeeds. Test, build or packaging failure does not publish; upload failure may
leave an incomplete draft.

## Recovery and corrections

After fixing an operational failure, a human reruns the failed tag workflow in
GitHub Actions. Reruns for one tag are serialized. An existing draft is resumed
only when its recorded target is the exact same commit SHA; its matching assets
are replaced during the upload and it is published after complete staging. Do
not manually publish an incomplete draft. A manually created draft with a branch
name instead of the exact SHA is refused; inspect and remove that unpublished
draft before rerunning if appropriate.

An already published release is refused without changing its assets. Correct a
published release with a **new version**, never by moving its tag or replacing
its assets. Humans own merging, tagging, recovery and the first release; this
workflow does not change Workflow Ledger publication or configuration.

See [README installation](../README.md#install) for binary and Go installation.
