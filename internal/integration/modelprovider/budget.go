package modelprovider

// ContextBudget is shared by admission, inference and compaction. The hard
// compaction boundary is exclusive: fixed input at that boundary cannot run,
// even in an empty conversation, and must be rejected before creating a Run.
type ContextBudget struct {
	Usable, SoftLimit, HardLimit int
}

func NewContextBudget(contextWindow, maxOutput int) ContextBudget {
	usable := max(0, contextWindow-maxOutput-max(4096, contextWindow/20))
	return ContextBudget{Usable: usable, SoftLimit: usable * 70 / 100, HardLimit: usable * 85 / 100}
}

func (budget ContextBudget) Fits(input int) bool {
	return budget.HardLimit > 0 && input < budget.HardLimit
}
