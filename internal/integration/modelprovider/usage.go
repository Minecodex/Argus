package modelprovider

import "errors"

var ErrInvalidUsage = errors.New("model usage is inconsistent")

// TokenUsage keeps provenance independently for each direction. Explicit zero
// is reported usage; an absent field must never silently become reported zero.
type TokenUsage struct {
	Input, Output, CachedInput int64
	cachedSource               string
	inputSource, outputSource  string
	invalid                    bool
}

func (u *TokenUsage) Observe(event Event) {
	if event.CachedInputKnown {
		u.CachedInput, u.cachedSource = event.CachedInput, "provider"
	}
	if event.InputKnown {
		u.Input, u.inputSource = event.Input, "provider"
	}
	if event.OutputKnown {
		u.Output, u.outputSource = event.Output, "provider"
	}
}

func (u *TokenUsage) Estimate(input, output int64) {
	if u.Invalid() {
		return
	}
	if u.inputSource == "" && input > 0 {
		u.Input, u.inputSource = input, "estimated"
	}
	if u.outputSource == "" && output > 0 {
		u.Output, u.outputSource = output, "estimated"
	}
}

func usageSource(source string) string {
	if source == "" {
		return "missing"
	}
	return source
}
func (u *TokenUsage) Invalidate() { u.invalid = true }

// Validate the assembled state, not just one update. Providers may send the
// cache breakdown before the total, and may revise totals before completion.
func (u TokenUsage) Validate() error {
	if u.invalid || u.Input < 0 || u.Output < 0 || u.CachedInput < 0 ||
		u.cachedSource == "provider" && (u.inputSource != "provider" || u.CachedInput > u.Input) {
		return ErrInvalidUsage
	}
	return nil
}
func (u TokenUsage) Invalid() bool { return u.Validate() != nil }
func (u TokenUsage) source(value string) string {
	if u.Invalid() {
		return "invalid"
	}
	return usageSource(value)
}
func (u TokenUsage) InputSource() string  { return u.source(u.inputSource) }
func (u TokenUsage) OutputSource() string { return u.source(u.outputSource) }
func (u TokenUsage) CachedSource() string { return u.source(u.cachedSource) }
func (u TokenUsage) CacheComplete() bool {
	return !u.Invalid() && u.cachedSource == "provider" && u.inputSource == "provider"
}
func (u TokenUsage) Complete() bool {
	return !u.Invalid() && u.inputSource == "provider" && u.outputSource == "provider"
}

func emitUsage(sink func(Event) error, kind, reason string, input, output, cached *int64) error {
	event := Event{Type: kind, StopReason: reason}
	if input != nil {
		if *input < 0 {
			return ErrInvalidUsage
		}
		event.Input, event.InputKnown = *input, true
	}
	if output != nil {
		if *output < 0 {
			return ErrInvalidUsage
		}
		event.Output, event.OutputKnown = *output, true
	}
	if cached != nil {
		if *cached < 0 || input != nil && *cached > *input {
			return ErrInvalidUsage
		}
		event.CachedInput, event.CachedInputKnown = *cached, true
	}
	return sink(event)
}
