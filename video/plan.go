// Package video contains the small, domain-specific state model used by a
// video generation application built on top of agentengine.
//
// The package deliberately does not generate media. Providers are exposed to
// the Agent as normal Toolbox tools; this package validates their structured
// plan and keeps media state independent from the conversation transcript.
package video

import (
	"fmt"
	"strings"
)

type ShotStatus string

const (
	ShotPending   ShotStatus = "pending"
	ShotRunning   ShotStatus = "running"
	ShotSucceeded ShotStatus = "succeeded"
	ShotFailed    ShotStatus = "failed"
	ShotLocked    ShotStatus = "locked"
)

type QCResult struct {
	Passed   bool     `json:"passed"`
	Issues   []string `json:"issues,omitempty"`
	Duration float32  `json:"duration,omitempty"`
}

type Shot struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	Prompt      string     `json:"prompt"`
	DurationSec float32    `json:"duration_sec"`
	Status      ShotStatus `json:"status"`
	AssetURI    string     `json:"asset_uri,omitempty"`
	Provider    string     `json:"provider,omitempty"`
	Model       string     `json:"model,omitempty"`
	QC          QCResult   `json:"qc,omitempty"`
}

type Plan struct {
	Title       string  `json:"title"`
	DurationSec float32 `json:"duration_sec"`
	AspectRatio string  `json:"aspect_ratio"`
	Language    string  `json:"language,omitempty"`
	Style       string  `json:"style,omitempty"`
	Shots       []Shot  `json:"shots"`
}

// Validate checks the invariants required before an expensive generation run.
func (p Plan) Validate() error {
	if strings.TrimSpace(p.Title) == "" {
		return fmt.Errorf("video title is required")
	}
	if p.DurationSec <= 0 {
		return fmt.Errorf("video duration must be positive")
	}
	if strings.TrimSpace(p.AspectRatio) == "" {
		return fmt.Errorf("video aspect ratio is required")
	}
	if len(p.Shots) == 0 {
		return fmt.Errorf("video plan must contain at least one shot")
	}
	var total float32
	seen := make(map[string]bool, len(p.Shots))
	for i, shot := range p.Shots {
		if strings.TrimSpace(shot.ID) == "" {
			return fmt.Errorf("shot %d has no id", i+1)
		}
		if seen[shot.ID] {
			return fmt.Errorf("duplicate shot id %q", shot.ID)
		}
		seen[shot.ID] = true
		if shot.DurationSec <= 0 {
			return fmt.Errorf("shot %q duration must be positive", shot.ID)
		}
		if strings.TrimSpace(shot.Description) == "" {
			return fmt.Errorf("shot %q description is required", shot.ID)
		}
		total += shot.DurationSec
	}
	if total > p.DurationSec*1.1 || total < p.DurationSec*0.9 {
		return fmt.Errorf("shot duration %.2fs does not match plan duration %.2fs", total, p.DurationSec)
	}
	return nil
}

// Retryable reports whether a failed shot can be regenerated without
// replacing a user-locked asset.
func (s Shot) Retryable() bool {
	return s.Status == ShotFailed
}

// Lock prevents later retries from replacing an approved asset.
func (s *Shot) Lock() error {
	if strings.TrimSpace(s.AssetURI) == "" {
		return fmt.Errorf("shot %q has no asset to lock", s.ID)
	}
	s.Status = ShotLocked
	return nil
}

// RetryFailed resets only failed shots and leaves successful or locked shots
// untouched, enabling cheap partial reruns.
func (p *Plan) RetryFailed() int {
	count := 0
	for i := range p.Shots {
		if p.Shots[i].Retryable() {
			p.Shots[i].Status = ShotPending
			p.Shots[i].QC = QCResult{}
			count++
		}
	}
	return count
}
