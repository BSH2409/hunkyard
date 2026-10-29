package focus

import "github.com/BSH2409/hunkyard/internal/session"

type (
	Focus          = session.Focus
	FocusKind      = session.FocusKind
	DiffScope      = session.DiffScope
	HunkyardJSON   = session.HunkyardJSON
	InheritedScope = session.InheritedScope
)

const (
	FocusWorkingTree   = session.FocusWorkingTree
	FocusRange         = session.FocusRange
	DiffScopeLayer     = session.DiffScopeLayer
	DiffScopeFullStack = session.DiffScopeFullStack
)
