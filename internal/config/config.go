package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration from environment variables.
type Config struct {
	URL           string
	Username      string
	Password      string
	Cookies       map[string]string
	Token         string
	ForcePush     bool
	GitHubActions bool
	TimeoutSec    int // HTTP timeout in seconds (default 30)

	// GitHub Actions metadata (used only in GHA summary).
	RefName      string
	EventName    string
	Actor        string
	ActorID      string
	TriggerActor string
	Repository   string
	SHA          string
	Workflow     string
	RunNumber    string
	RunID        string
	BeijingTime  string
	StepSummary  string
}

// Load reads configuration from environment variables.
// In non-GitHub-Actions environments, ForcePush defaults to true
// so pushes are always sent when the script runs locally.
func Load() *Config {
	ga := os.Getenv("GITHUB_ACTIONS") == "true"
	forceRaw := os.Getenv("FORCE_PUSH_MESSAGE")
	forcePush := forceRaw == "True"
	if !ga {
		forcePush = true
	}
	timeoutSec := 30
	if v := os.Getenv("TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeoutSec = n
		}
	}
	return &Config{
		URL:           strings.TrimRight(os.Getenv("URL"), "/"),
		Username:      os.Getenv("USERNAME"),
		Password:      os.Getenv("PASSWORD"),
		Cookies:       parseCookies(os.Getenv("COOKIES")),
		Token:         os.Getenv("TOKEN"),
		ForcePush:     forcePush,
		GitHubActions: ga,
		TimeoutSec:    timeoutSec,
		RefName:       os.Getenv("GITHUB_REF_NAME"),
		EventName:     os.Getenv("GITHUB_EVENT_NAME"),
		Actor:         os.Getenv("GITHUB_ACTOR"),
		ActorID:       os.Getenv("GITHUB_ACTOR_ID"),
		TriggerActor:  os.Getenv("GITHUB_TRIGGERING_ACTOR"),
		Repository:    os.Getenv("REPOSITORY_NAME"),
		SHA:           os.Getenv("GITHUB_SHA"),
		Workflow:      os.Getenv("GITHUB_WORKFLOW"),
		RunNumber:     os.Getenv("GITHUB_RUN_NUMBER"),
		RunID:         os.Getenv("GITHUB_RUN_ID"),
		BeijingTime:   os.Getenv("BEIJING_TIME"),
		StepSummary:   os.Getenv("GITHUB_STEP_SUMMARY"),
	}
}

// parseCookies converts a COOKIES env string into a map.
// Supports two formats:
//   - JSON:   {"JSESSIONID":"...","route":"..."}
//   - String: JSESSIONID=...; route=...
//
// Returns nil if empty or unparseable.
func parseCookies(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	// Try JSON first.
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err == nil && len(m) > 0 {
		return m
	}
	// Fallback: semicolon-delimited name=value pairs.
	m = make(map[string]string)
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			m[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}
