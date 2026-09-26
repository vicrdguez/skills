package ledger

import (
	"slices"
	"strconv"
	"strings"
)

// Selection values beyond the recorded lifecycles and Claim phases.
const (
	// ClaimNone selects Slices whose readable state records no Claim.
	ClaimNone = "none"

	GroupByProposal  = "proposal"
	GroupByLifecycle = "lifecycle"
)

// Lifecycles lists the canonical lifecycles in workflow order.
var Lifecycles = []string{ReadyForImplementation, AwaitingReview, Rework, NeedsHuman, ReadyForMerge, Merged, Superseded}

// SliceQuery selects Slices by criteria that must all hold. An empty
// criterion selects every Slice along its dimension. Lifecycle and Claim are
// independent: a Claim phase never selects a lifecycle, and no criterion
// changes eligibility.
type SliceQuery struct {
	// Project narrows the scope to one Project; empty searches every Project.
	Project string `json:"project,omitempty"`
	// IncludeArchived adds the Slices of archived Proposals to the scope.
	IncludeArchived bool `json:"include_archived"`
	// Lifecycles selects any of the given recorded lifecycles.
	Lifecycles []string `json:"lifecycles,omitempty"`
	// Claims selects any of the given Claim phases, or ClaimNone.
	Claims []string `json:"claims,omitempty"`
	// Text matches, ignoring case, part of a Slice's project/proposal/slice
	// identity or its recorded title. Document bodies and reports are never
	// searched.
	Text string `json:"text,omitempty"`
	// GroupBy groups each Project's matches by Proposal (the default) or by
	// lifecycle.
	GroupBy string `json:"group_by"`
}

// SliceSearch answers one SliceQuery at one committed revision, grouped by
// Project. Matched Slices satisfy every criterion through readable facts.
// Undecided Slices are excluded by no readable fact, but an unknown
// lifecycle, Claim, or title leaves a criterion undecided, so they are
// neither matches nor excluded. Incomplete is set whenever undecided Slices
// or unknown membership keep the result from being complete; a result that
// is empty and complete matches nothing.
type SliceSearch struct {
	Revision    string          `json:"revision"`
	Query       SliceQuery      `json:"query"`
	Facets      SliceFacets     `json:"facets"`
	Matched     int             `json:"matched"`
	Undecided   int             `json:"undecided"`
	Projects    []ProjectSlices `json:"projects"`
	Incomplete  bool            `json:"incomplete"`
	Diagnostics []Diagnostic    `json:"diagnostics,omitempty"`
}

// SliceFacets count, for each lifecycle and Claim value, the Slices in scope
// the query would match if that dimension selected only that value. The
// unknown counts are the Slices whose unknown facts would leave that
// dimension undecided.
type SliceFacets struct {
	Lifecycles       map[string]int `json:"lifecycles"`
	UnknownLifecycle int            `json:"unknown_lifecycle"`
	Claims           map[string]int `json:"claims"`
	UnknownClaim     int            `json:"unknown_claim"`
}

// ProjectSlices is one Project's part of a search. Diagnostics disclose the
// unreadable membership in its scope.
type ProjectSlices struct {
	Name        string       `json:"name"`
	Repository  string       `json:"repository,omitempty"`
	Groups      []SliceGroup `json:"groups"`
	Undecided   []SliceMatch `json:"undecided"`
	Incomplete  bool         `json:"incomplete"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// SliceGroup holds the matches of one Proposal, or of one lifecycle.
// UnknownLifecycle groups the matches whose lifecycle is unreadable or
// unsupported.
type SliceGroup struct {
	Proposal         string       `json:"proposal,omitempty"`
	Archived         bool         `json:"archived,omitempty"`
	Lifecycle        string       `json:"lifecycle,omitempty"`
	UnknownLifecycle bool         `json:"unknown_lifecycle,omitempty"`
	Slices           []SliceMatch `json:"slices"`
}

// SliceMatch is one selected Slice with its Proposal.
type SliceMatch struct {
	Proposal string `json:"proposal"`
	Archived bool   `json:"archived"`
	SliceSummary
}

// decision is a criterion's three-valued answer; combining criteria takes the
// least of them.
type decision int

const (
	excluded decision = iota
	undecided
	included
)

func decide(holds bool) decision {
	if holds {
		return included
	}
	return excluded
}

// FindSlices selects Slices across the query's scope. Unknown facts are never
// inferred: they leave a Slice undecided instead of matching or excluding it.
func (v *Snapshot) FindSlices(query SliceQuery) (*SliceSearch, error) {
	query, err := v.normalize(query)
	if err != nil {
		return nil, err
	}
	var projects []*projectTree
	if query.Project != "" {
		projects = []*projectTree{v.projects[query.Project]}
	} else {
		for _, name := range v.ProjectNames() {
			projects = append(projects, v.projects[name])
		}
	}
	reads, err := v.read(projects, query.IncludeArchived)
	if err != nil {
		return nil, err
	}
	search := &SliceSearch{
		Revision: v.Revision, Query: query, Projects: []ProjectSlices{},
		Facets: SliceFacets{Lifecycles: map[string]int{}, Claims: map[string]int{}},
	}
	if query.Project == "" {
		search.Diagnostics = v.invalid
	}
	for index, read := range reads {
		result := ProjectSlices{Name: read.name, Repository: read.repository, Groups: []SliceGroup{}, Undecided: []SliceMatch{}}
		result.Diagnostics = append(result.Diagnostics, projects[index].diagnostics...)
		var matches []SliceMatch
		for _, proposal := range read.proposals {
			result.Diagnostics = append(result.Diagnostics, proposal.tree.membership(read.name+"/"+proposal.tree.name)...)
			for _, slice := range proposal.slices {
				match := SliceMatch{Proposal: proposal.tree.name, Archived: proposal.tree.archived, SliceSummary: slice.summary(proposal.tree.name)}
				text := query.text(read.name+"/"+match.Item, slice)
				lifecycle, claim := query.lifecycle(slice), query.claim(slice)
				if min(text, claim) == included {
					search.Facets.countLifecycle(slice)
				}
				if min(text, lifecycle) == included {
					search.Facets.countClaim(slice)
				}
				switch min(text, lifecycle, claim) {
				case included:
					matches = append(matches, match)
				case undecided:
					result.Undecided = append(result.Undecided, match)
				}
			}
		}
		result.Groups = group(matches, query.GroupBy)
		result.Incomplete = len(result.Diagnostics) > 0 || len(result.Undecided) > 0
		search.Matched += len(matches)
		search.Undecided += len(result.Undecided)
		search.Incomplete = search.Incomplete || result.Incomplete
		if query.Project != "" || len(matches) > 0 || result.Incomplete {
			search.Projects = append(search.Projects, result)
		}
	}
	search.Incomplete = search.Incomplete || len(search.Diagnostics) > 0
	return search, nil
}

// normalize validates a query's selections and fills its defaults.
func (v *Snapshot) normalize(query SliceQuery) (SliceQuery, error) {
	if query.Project != "" {
		if _, err := v.project(query.Project); err != nil {
			return query, err
		}
	}
	for _, lifecycle := range query.Lifecycles {
		if !knownLifecycle(lifecycle) {
			return query, refuse("unsupported lifecycle selection "+strconv.Quote(lifecycle), "select one of "+strings.Join(Lifecycles, ", "))
		}
	}
	for _, claim := range query.Claims {
		if claim != ImplementPhase && claim != WatchdogPhase && claim != ClaimNone {
			return query, refuse("unsupported Claim selection "+strconv.Quote(claim), "select "+ImplementPhase+", "+WatchdogPhase+", or "+ClaimNone)
		}
	}
	switch query.GroupBy {
	case "":
		query.GroupBy = GroupByProposal
	case GroupByProposal, GroupByLifecycle:
	default:
		return query, refuse("unsupported grouping "+strconv.Quote(query.GroupBy), "group by "+GroupByProposal+" or "+GroupByLifecycle)
	}
	query.Text = strings.TrimSpace(query.Text)
	return query, nil
}

func (q SliceQuery) text(identity string, slice sliceRead) decision {
	needle := strings.ToLower(q.Text)
	switch {
	case needle == "" || strings.Contains(strings.ToLower(identity), needle):
		return included
	case !slice.readable:
		return undecided
	}
	return decide(strings.Contains(strings.ToLower(slice.state.Title), needle))
}

func (q SliceQuery) lifecycle(slice sliceRead) decision {
	lifecycle, known := slice.lifecycle()
	switch {
	case len(q.Lifecycles) == 0:
		return included
	case !known:
		return undecided
	}
	return decide(slices.Contains(q.Lifecycles, lifecycle))
}

func (q SliceQuery) claim(slice sliceRead) decision {
	claim, known := slice.claim()
	switch {
	case len(q.Claims) == 0:
		return included
	case !known:
		return undecided
	}
	return decide(slices.Contains(q.Claims, claim))
}

// lifecycle is the Slice's readable, supported lifecycle.
func (r sliceRead) lifecycle() (string, bool) {
	if !r.readable || !knownLifecycle(r.state.State) {
		return "", false
	}
	return r.state.State, true
}

// claim is the Slice's readable, supported Claim phase, or ClaimNone.
func (r sliceRead) claim() (string, bool) {
	switch {
	case !r.readable:
		return "", false
	case r.state.Claim == nil:
		return ClaimNone, true
	case r.state.Claim.Phase == ImplementPhase || r.state.Claim.Phase == WatchdogPhase:
		return r.state.Claim.Phase, true
	}
	return "", false
}

func (f *SliceFacets) countLifecycle(slice sliceRead) {
	if lifecycle, known := slice.lifecycle(); known {
		f.Lifecycles[lifecycle]++
	} else {
		f.UnknownLifecycle++
	}
}

func (f *SliceFacets) countClaim(slice sliceRead) {
	if claim, known := slice.claim(); known {
		f.Claims[claim]++
	} else {
		f.UnknownClaim++
	}
}

// group orders matches by Proposal as recorded, or by lifecycle in workflow
// order with unknown lifecycles last.
func group(matches []SliceMatch, by string) []SliceGroup {
	groups := []SliceGroup{}
	if by == GroupByProposal {
		for _, match := range matches {
			last := len(groups) - 1
			if last < 0 || groups[last].Proposal != match.Proposal || groups[last].Archived != match.Archived {
				groups = append(groups, SliceGroup{Proposal: match.Proposal, Archived: match.Archived})
				last++
			}
			groups[last].Slices = append(groups[last].Slices, match)
		}
		return groups
	}
	for _, lifecycle := range append(slices.Clone(Lifecycles), "") {
		bucket := SliceGroup{Lifecycle: lifecycle, UnknownLifecycle: lifecycle == ""}
		for _, match := range matches {
			if known := match.Readable && knownLifecycle(match.Lifecycle); (known && match.Lifecycle == lifecycle) || (!known && lifecycle == "") {
				bucket.Slices = append(bucket.Slices, match)
			}
		}
		if len(bucket.Slices) > 0 {
			groups = append(groups, bucket)
		}
	}
	return groups
}
