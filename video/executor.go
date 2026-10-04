package video

import (
	"context"
	"fmt"
)

// Provider is the small adapter an application implements for its media
// backend. The executor owns orchestration; the provider owns model APIs and
// object storage.
type Provider interface {
	Generate(context.Context, Shot) (assetURI string, err error)
	Compose(context.Context, Plan) (assetURI string, err error)
	Probe(context.Context, string) (QCResult, error)
}

type EventKind string

const (
	ShotStarted          EventKind = "shot_started"
	ShotCompleted        EventKind = "shot_completed"
	ShotGenerationFailed EventKind = "shot_failed"
	RenderStarted        EventKind = "render_started"
	QualityCheck         EventKind = "quality_check"
	Completed            EventKind = "completed"
)

type Event struct {
	Kind      EventKind
	ShotID    string
	AssetURI  string
	Message   string
	Completed int
	Total     int
}

type Executor struct {
	Provider Provider
	// MaxAttempts applies only to failed, unlocked shots.
	MaxAttempts int
}

// Run generates pending/failed shots, composes them, and verifies the final
// asset. Locked and already successful shots are reused. Events are optional.
func (e Executor) Run(ctx context.Context, plan *Plan, events chan<- Event) (string, error) {
	if e.Provider == nil {
		return "", fmt.Errorf("video provider is required")
	}
	if plan == nil {
		return "", fmt.Errorf("video plan is required")
	}
	if err := plan.Validate(); err != nil {
		return "", err
	}
	attempts := e.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	completed := 0
	for i := range plan.Shots {
		shot := &plan.Shots[i]
		if shot.Status == ShotLocked || (shot.Status == ShotSucceeded && shot.AssetURI != "") {
			completed++
			continue
		}
		if shot.Status != ShotPending && shot.Status != ShotFailed {
			shot.Status = ShotPending
		}
		send(events, Event{Kind: ShotStarted, ShotID: shot.ID, Completed: completed, Total: len(plan.Shots)})
		var lastErr error
		for attempt := 0; attempt < attempts; attempt++ {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			shot.Status = ShotRunning
			asset, err := e.Provider.Generate(ctx, *shot)
			if err == nil && asset != "" {
				shot.AssetURI = asset
				shot.Status = ShotSucceeded
				lastErr = nil
				break
			}
			lastErr = err
			if lastErr == nil {
				lastErr = fmt.Errorf("provider returned an empty asset URI")
			}
		}
		if lastErr != nil {
			shot.Status = ShotFailed
			send(events, Event{Kind: ShotGenerationFailed, ShotID: shot.ID, Message: lastErr.Error(), Completed: completed, Total: len(plan.Shots)})
			return "", fmt.Errorf("shot %s: %w", shot.ID, lastErr)
		}
		completed++
		send(events, Event{Kind: ShotCompleted, ShotID: shot.ID, AssetURI: shot.AssetURI, Completed: completed, Total: len(plan.Shots)})
	}

	send(events, Event{Kind: RenderStarted, Completed: completed, Total: len(plan.Shots)})
	finalURI, err := e.Provider.Compose(ctx, *plan)
	if err != nil {
		return "", fmt.Errorf("compose video: %w", err)
	}
	if finalURI == "" {
		return "", fmt.Errorf("compose video returned an empty asset URI")
	}
	qc, err := e.Provider.Probe(ctx, finalURI)
	if err != nil {
		return "", fmt.Errorf("quality check: %w", err)
	}
	send(events, Event{Kind: QualityCheck, AssetURI: finalURI, Message: qcMessage(qc)})
	if !qc.Passed {
		return "", fmt.Errorf("quality check failed: %s", qcMessage(qc))
	}
	send(events, Event{Kind: Completed, AssetURI: finalURI, Completed: completed, Total: len(plan.Shots)})
	return finalURI, nil
}

func qcMessage(qc QCResult) string {
	if len(qc.Issues) == 0 {
		return "passed"
	}
	return fmt.Sprintf("%v", qc.Issues)
}

func send(events chan<- Event, event Event) {
	if events != nil {
		events <- event
	}
}
