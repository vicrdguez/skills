Status: interrupted

The wait for {{template "outcome-phase" .Phase}} work was interrupted, and a Claim may have been acquired before it stopped. Run `{{.StatusCommand}}` to see which Work Items hold a Claim. Tell the user the wait was interrupted and what that shows, and stop.
