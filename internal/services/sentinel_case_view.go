package services

// SentinelRunViewFor returns the backward-compatible run payload with the
// Case attachment surfaced as a first-class response field.
func SentinelRunViewFor(run SentinelRun) SentinelRunView {
	return SentinelRunView{SentinelRun: run, CaseID: sentinelCaseIDFromRunEvidence(run.Evidence)}
}
