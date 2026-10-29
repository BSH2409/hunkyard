package comment

import (
	"github.com/BSH2409/hunkyard/internal/review"
	"github.com/BSH2409/hunkyard/internal/session"
)

type (
	HunkyardJSON     = session.HunkyardJSON
	HunkyardJSONFile = session.HunkyardJSONFile
	Comment          = session.Comment
	DOMAnchor        = session.DOMAnchor
	Reply            = session.Reply
	Focus            = session.Focus
	inheritedScope   = session.InheritedScope
)

const (
	FocusRange         = session.FocusRange
	FocusWorkingTree   = session.FocusWorkingTree
	DiffScopeLayer     = session.DiffScopeLayer
	DiffScopeFullStack = session.DiffScopeFullStack
)

var (
	reviewPathsFor   = session.ReviewPathsFor
	loadHunkyardJSON = review.LoadHunkyardJSON
	saveHunkyardJSON = review.SaveHunkyardJSON
)
