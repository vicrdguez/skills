#!/usr/bin/env bash
set -euo pipefail

# Run from the tagged checkout. Only the workflow supplies publication authority.
tag=${RELEASE_TAG:?RELEASE_TAG is required}
if [[ ! $tag =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  printf 'Not a supported stable tag: %s\n' "$tag" >&2
  exit 1
fi
commit=$(git rev-parse HEAD)
[[ $(git rev-parse "refs/tags/$tag^{commit}") == "$commit" ]]
git fetch origin main
git merge-base --is-ancestor "$commit" FETCH_HEAD

# Listing distinguishes absence from transport failure, including private drafts.
release=$(gh api --paginate 'repos/{owner}/{repo}/releases' \
  --jq ".[] | select(.tag_name == \"$tag\") | [.draft, .target_commitish] | @tsv")
if [[ -n $release && $release != $'true\t'"$commit" ]]; then
  printf 'Refusing a published release or a draft for another commit: %s\n' "$tag" >&2
  exit 1
fi

go test ./...
artifacts=$(mktemp -d)
trap 'rm -rf "$artifacts"' EXIT
for os in darwin linux; do
  for arch in amd64 arm64; do
    CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build \
      -ldflags "-X main.releaseVersion=$tag" -o "$artifacts/skl" ./cmd/skl
    COPYFILE_DISABLE=1 tar -czf "$artifacts/skl_${tag}_${os}_${arch}.tar.gz" -C "$artifacts" skl
  done
done
rm "$artifacts/skl"
(cd "$artifacts" && shasum -a 256 ./*.tar.gz > checksums.txt)

if [[ -z $release ]]; then
  gh release create "$tag" --draft --verify-tag --target "$commit" --generate-notes
fi
# Complete preparation precedes all uploads. A failed/partial upload stays draft.
gh release upload "$tag" "$artifacts"/*.tar.gz "$artifacts/checksums.txt" --clobber
gh release edit "$tag" --draft=false
