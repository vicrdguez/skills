package ledger

import "context"

// PullReadiness identifies an established attachment at one final approved
// head. It deliberately carries neither prose nor creation inputs. Current
// checks local authority and actual remote source before further effects.
type PullReadiness struct {
	Number             int
	Branch, Head, Base string
	Current            func() error `json:"-"`
}

// ReadinessForge reconciles only native readiness on an exact attachment.
type ReadinessForge interface {
	ReadyPull(context.Context, PullReadiness) error
}
