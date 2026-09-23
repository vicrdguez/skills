package ledger

import "context"

// PublicationStatus is the forge receipt protocol. Only these named statuses
// establish an observed outcome; an empty or unknown status remains pending.
type PublicationStatus string

const (
	PublicationPublished        = "published"
	PublicationAlreadySatisfied = "already-satisfied"
	PublicationPending          = "pending"
	PublicationProseNeeded      = "prose-needed"
	PublicationStale            = "stale"
	PublicationAmbiguous        = "ambiguous"
	FindingSatisfied            = "satisfied"
	FindingInvalid              = "invalid"
	FindingUnresolved           = "unresolved"
)

// PublicationView identifies the selected committed presentation without
// retaining its public prose. Token excludes unrelated ledger changes.
type PublicationView struct {
	Project    string            `json:"project"`
	Repository string            `json:"repository"`
	Proposal   string            `json:"proposal"`
	Item       string            `json:"item"`
	Kind       string            `json:"kind"`
	Token      string            `json:"token"`
	Title      string            `json:"title"`
	Branch     string            `json:"branch,omitempty"`
	State      string            `json:"state"`
	Source     SourceRevisions   `json:"source"`
	Phase      string            `json:"phase,omitempty"`
	Report     *Reference        `json:"report,omitempty"`
	Contracts  []Reference       `json:"contracts"`
	Attachment *ForgeAttachment  `json:"attachment,omitempty"`
	Parent     *ForgeAttachment  `json:"parent,omitempty"`
	Children   []ForgeAttachment `json:"children,omitempty"`
	Status     string            `json:"status"`
	Detail     string            `json:"detail,omitempty"`
	BodyPath   string            `json:"body_path,omitempty"`
}

// SelectedFinding is deliberately public prose authorized for one reviewed
// code anchor. It is never populated by exporting private report text.
type SelectedFinding struct {
	ID     string `json:"id"`
	Body   string `json:"body"`
	Commit string `json:"commit"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Side   string `json:"side"`
}

type FindingPublication struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type PublicationRequest struct {
	Item     string
	Kind     string
	View     string
	BodyPath string
	Findings []SelectedFinding
}

type PublicationResult struct {
	Status   string               `json:"status"`
	Detail   string               `json:"detail,omitempty"`
	View     PublicationView      `json:"view"`
	Findings []FindingPublication `json:"findings,omitempty"`
}

// RecoveryPresentation crosses the forge seam with public material and
// deterministic guards; it carries no private Contract or Phase Report prose.
type RecoveryPresentation struct {
	Kind               string
	Number             int
	Title              string
	Body               *string
	Branch             string
	Head               string
	Reviewed           string
	SourceRoot         string
	Target             string
	Approved           bool
	OriginalBodySHA256 string
	MayHaveCreated     bool
	ObserveOnly        bool
	Parent             *ForgeAttachment
	Children           []ForgeAttachment
	Findings           []SelectedFinding
	Guard              func() error
	PrepareSource      func(observedHead string) error
}

type RecoveryReceipt struct {
	Number   int
	Status   PublicationStatus
	Detail   string
	Findings []FindingPublication
}

type RecoveryForge interface {
	RecoverPresentation(context.Context, RecoveryPresentation) (RecoveryReceipt, error)
}
