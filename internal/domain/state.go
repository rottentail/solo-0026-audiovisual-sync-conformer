package domain

const (
	TakeStateValidating = "validating"
	TakeStateAdmitted   = "admitted"

	AnalysisStateQueued    = "queued"
	AnalysisStateDetecting = "detecting"
	AnalysisStateReady     = "ready"
	AnalysisStateFailed    = "failed"

	ConformStateQueued     = "queued"
	ConformStateRendering  = "rendering"
	ConformStateCommitting = "committing"
	ConformStateCompleted  = "completed"
	ConformStateFailed     = "failed"
)

func CanAdmitTake(next string) bool {
	return next == TakeStateAdmitted
}

func CanAdvanceAnalysis(current, next string) bool {
	switch current {
	case AnalysisStateQueued:
		return next == AnalysisStateDetecting || next == AnalysisStateFailed
	case AnalysisStateDetecting:
		return next == AnalysisStateReady || next == AnalysisStateFailed
	default:
		return false
	}
}

func AnalysisTerminal(state string) bool {
	return state == AnalysisStateReady || state == AnalysisStateFailed
}

func CanAdvanceConform(current, next string) bool {
	switch current {
	case ConformStateQueued:
		return next == ConformStateRendering || next == ConformStateFailed
	case ConformStateRendering:
		return next == ConformStateCommitting || next == ConformStateFailed
	case ConformStateCommitting:
		return next == ConformStateCompleted || next == ConformStateFailed
	default:
		return false
	}
}

func ConformTerminal(state string) bool {
	return state == ConformStateCompleted || state == ConformStateFailed
}
