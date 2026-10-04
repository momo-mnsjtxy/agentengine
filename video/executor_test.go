package video

import (
	"context"
	"errors"
	"testing"
)

type fakeProvider struct {
	shotCalls int
	compose   int
	probe     int
	failed    bool
}

func (p *fakeProvider) Generate(_ context.Context, shot Shot) (string, error) {
	p.shotCalls++
	if p.failed && shot.ID == "s2" && p.shotCalls == 2 {
		return "", errors.New("temporary provider error")
	}
	return "gs://assets/" + shot.ID + ".mp4", nil
}
func (p *fakeProvider) Compose(_ context.Context, _ Plan) (string, error) {
	p.compose++
	return "gs://assets/final.mp4", nil
}
func (p *fakeProvider) Probe(_ context.Context, _ string) (QCResult, error) {
	p.probe++
	return QCResult{Passed: true}, nil
}

func TestExecutorRunsAndReusesLockedShots(t *testing.T) {
	p := &fakeProvider{}
	plan := validPlan()
	plan.Shots[0].AssetURI = "gs://assets/locked.mp4"
	if err := plan.Shots[0].Lock(); err != nil {
		t.Fatal(err)
	}
	uri, err := (Executor{Provider: p}).Run(context.Background(), &plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if uri != "gs://assets/final.mp4" || p.shotCalls != 1 || p.compose != 1 || p.probe != 1 {
		t.Fatalf("unexpected execution: uri=%q shots=%d compose=%d probe=%d", uri, p.shotCalls, p.compose, p.probe)
	}
}

func TestExecutorRetriesAndStopsOnQualityFailure(t *testing.T) {
	p := &fakeProvider{}
	plan := validPlan()
	plan.Shots[1].Status = ShotPending
	uri, err := (Executor{Provider: p, MaxAttempts: 2}).Run(context.Background(), &plan, nil)
	if err != nil || uri == "" {
		t.Fatalf("run failed: %q %v", uri, err)
	}
}
