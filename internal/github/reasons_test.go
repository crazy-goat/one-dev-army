package github

import "testing"

// allReasons lists every StageChangeReason constant for exhaustive testing.
var allReasons = []StageChangeReason{
	ReasonManualApprove,
	ReasonManualReject,
	ReasonManualRetry,
	ReasonManualRetryFresh,
	ReasonManualBlock,
	ReasonManualUnblock,
	ReasonManualDecline,
	ReasonManualMerge,
	ReasonManualMergeFailed,
	ReasonWorkerAlreadyDone,
	ReasonWorkerFailed,
	ReasonWorkerApprove,
	ReasonWorkerCompletedAnalysis,
	ReasonWorkerCompletedCoding,
	ReasonWorkerCompletedCodeReview,
	ReasonWorkerCompletedCreatePR,
	ReasonWorkerNeedsUser,
	ReasonWorkerBlocked,
	ReasonWorkerStageUpdate,
	ReasonSyncInitial,
	ReasonSyncPeriodic,
	ReasonSyncManual,
}

func TestStageChangeReasonLabelNonEmpty(t *testing.T) {
	for _, r := range allReasons {
		if r.Label() == "" {
			t.Errorf("StageChangeReason %q has empty Label()", r)
		}
	}
}

func TestStageChangeReasonStringNonEmpty(t *testing.T) {
	for _, r := range allReasons {
		s := r.String()
		if s == "" {
			t.Errorf("StageChangeReason %q has empty String()", r)
		}
		// String() should NOT equal Label() — it should be a human-readable description
		if s == r.Label() {
			t.Errorf("StageChangeReason %q: String() == Label() (%q) — missing description in switch", r, s)
		}
	}
}

func TestStageChangeReasonLabelRoundTrip(t *testing.T) {
	for _, r := range allReasons {
		// Label() should return the same value as the constant itself
		if StageChangeReason(r.Label()) != r {
			t.Errorf("StageChangeReason %q: Label() round-trip failed", r)
		}
	}
}

func TestStageChangeReasonUnknownFallback(t *testing.T) {
	unknown := StageChangeReason("some_unknown_reason")
	if unknown.String() != "some_unknown_reason" {
		t.Errorf("Unknown reason String() should fall back to raw value, got %q", unknown.String())
	}
	if unknown.Label() != "some_unknown_reason" {
		t.Errorf("Unknown reason Label() should return raw value, got %q", unknown.Label())
	}
}

func TestStageChangeReasonUniqueLabels(t *testing.T) {
	seen := make(map[string]StageChangeReason)
	for _, r := range allReasons {
		label := r.Label()
		if existing, ok := seen[label]; ok {
			t.Errorf("Duplicate label %q: used by both %q and %q", label, existing, r)
		}
		seen[label] = r
	}
}
