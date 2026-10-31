package preferences

import (
	"strconv"
	"strings"
)

// Message pools for terminal feedback while Hunkyard works.
var (
	spinnerVerbs = []string{"Yarding preferences…", "Rummaging through hunks…", "Hunkering down…", "Picking a few nits…", "Giving junk another diff…", "Making every hunk count…", "Rebasing on your taste…", "Raking the yard…", "Junk in, rules out…"}

	loadingMessages = []string{"Rummaging for your preferences…", "Checking your stash…", "Loading your nit list…", "Pulling from the yard…"}

	capturingMessages = []string{"Stashing that correction…", "Giving that nit a home…", "Taking the hunk hint…", "Turning scrap into wisdom…"}

	afterHookNotes = []string{"Salvaged {n} preferences.", "{n} yard rules aboard.", "{n} lessons out of mothballs.", "{n} nits, freshly picked."}

	maintenanceRunningMessages = []string{"Tidying the Preference Yard…", "Taking out the junk…", "Weeding the patch…"}

	maintenanceOverdueMessages = []string{"Yard overdue for a rake.", "Scrap sorter jammed.", "{n} scraps waiting to be sorted."}

	reviewWaitingMessages = []string{"Your hunk of the work.", "Got a nit to pick?", "Fresh hunks. Your call.", "The diff is yours."}

	spinnerTips = []string{"Your nits become yard rules.", "Teach the yard your style.", "Corrections stick here. Try one."}
)

// pickMessage chooses a pool entry deterministically from seed, so a retried
// hook shows the same line.
func pickMessage(pool []string, seed string) string {
	sum := 0
	for _, c := range digest(seed)[:8] {
		sum = sum*16 + int(c%16)
	}
	return pool[sum%len(pool)]
}

func fillCount(message string, n int) string {
	return strings.ReplaceAll(message, "{n}", strconv.Itoa(n))
}
