package setup

// githubPull describes pull requests read and written by ledger-backed delivery publication.
type githubPull struct {
	githubIssue
	Merged    bool   `json:"merged"`
	Mergeable *bool  `json:"mergeable"`
	NodeID    string `json:"node_id"`
	Draft     bool   `json:"draft"`
	MergedAt  string `json:"merged_at"`
	MergeSHA  string `json:"merge_commit_sha"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
	AuthorAssociation string `json:"author_association"`
	Head              struct {
		Ref  string `json:"ref"`
		SHA  string `json:"sha"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"head"`
	Base struct {
		Ref  string `json:"ref"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"base"`
}
