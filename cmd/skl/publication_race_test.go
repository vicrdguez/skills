package main

import (
	"fmt"
	"strings"
	"testing"
)

// GitHub cannot atomically bind a body PATCH to a source SHA. Recovery must
// detect movement, undo only its own observed body, and preserve newer prose.
func TestPublicationCLIBodyPatchPreservesMovedSource(t *testing.T) {
	for _, newerBody := range []bool{false, true} {
		t.Run(fmt.Sprint(newerBody), func(t *testing.T) {
			u := newPublicationPendingUpdate(t, "body-source-race")
			priorBody := u.forge.pull(u.number).Body
			wanted := priorBody
			if newerBody {
				wanted = "newer author's presentation\n"
			}
			moved := false
			u.forge.setAfterMutation(func(method, path string) {
				if method != "PATCH" || moved {
					return
				}
				moved = true
				// This hook holds the fixture lock after the body mutation.
				u.forge.branchRemote = ""
				u.forge.pulls[u.number].Head = strings.Repeat("9", 40)
				if newerBody {
					u.forge.pulls[u.number].Body = wanted
				}
			})
			out, err := newPublicationApp(t, u.forge).publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
				"--item", u.item, "--kind", "pull", "--format", "json")
			if err != nil {
				t.Fatal(err)
			}
			if out.Status != "pending" || u.forge.pull(u.number).Body != wanted {
				t.Fatalf("body/source race: status=%s body=%q want=%q", out.Status, u.forge.pull(u.number).Body, wanted)
			}
			if u.forge.count("POST", "/graphql") != 0 {
				t.Fatal("published readiness after observing moved source")
			}
			patches := 2
			if newerBody {
				patches = 1
			}
			if len(u.forge.pullPatches) != patches {
				t.Fatalf("body writes = %d, want %d", len(u.forge.pullPatches), patches)
			}
		})
	}
}
