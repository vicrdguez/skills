Status: {{.Status}}

{{template "outcome-previous" .Previous}}{{template "outcome-merges" .Merges}}No Work Item became ready for {{template "outcome-phase" .Phase}} during the wait, and no Claim was acquired. Tell the user no Work Item is ready, and stop.
