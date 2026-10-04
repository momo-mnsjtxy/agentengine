package video

import "testing"

func validPlan() Plan {
	return Plan{Title: "demo", DurationSec: 10, AspectRatio: "16:9", Shots: []Shot{
		{ID: "s1", Description: "opening", DurationSec: 5, Status: ShotSucceeded},
		{ID: "s2", Description: "ending", DurationSec: 5, Status: ShotFailed},
	}}
}

func TestPlanValidate(t *testing.T) {
	if err := validPlan().Validate(); err != nil {
		t.Fatalf("valid plan rejected: %v", err)
	}
	bad := validPlan()
	bad.Shots[1].ID = "s1"
	if err := bad.Validate(); err == nil {
		t.Fatal("duplicate shot id accepted")
	}
}

func TestRetryFailedPreservesLockedAndSuccessfulShots(t *testing.T) {
	p := validPlan()
	p.Shots[0].AssetURI = "gs://bucket/s1.mp4"
	if err := p.Shots[0].Lock(); err != nil {
		t.Fatal(err)
	}
	if got := p.RetryFailed(); got != 1 {
		t.Fatalf("reset %d shots, want 1", got)
	}
	if p.Shots[0].Status != ShotLocked || p.Shots[1].Status != ShotPending {
		t.Fatalf("unexpected statuses: %s, %s", p.Shots[0].Status, p.Shots[1].Status)
	}
}
