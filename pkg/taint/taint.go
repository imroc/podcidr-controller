package taint

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// TaintRule represents a taint matching rule
type TaintRule struct {
	Key    string              // Required
	Value  *string             // Optional, nil matches any value
	Effect *corev1.TaintEffect // Optional, nil matches any effect
}

// TaintRemover handles removing matching taints from nodes
type TaintRemover struct {
	rules []TaintRule
}

// NewTaintRemover parses config string to create TaintRemover
// Input format: "key1,key2:NoSchedule,key3=value:NoExecute"
// Returns nil if config is empty
func NewTaintRemover(config string) (*TaintRemover, error) {
	config = strings.TrimSpace(config)
	if config == "" {
		return nil, nil
	}

	parts := strings.Split(config, ",")
	rules := make([]TaintRule, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		rule, err := parseRule(part)
		if err != nil {
			return nil, fmt.Errorf("invalid taint rule %q: %w", part, err)
		}
		rules = append(rules, rule)
	}

	if len(rules) == 0 {
		return nil, nil
	}

	return &TaintRemover{rules: rules}, nil
}

// parseRule parses a single taint rule string
// Supported formats:
//   - "key" - matches any taint with this key
//   - "key:effect" - matches taint with this key and effect
//   - "key=value:effect" - exact match of key, value, and effect
func parseRule(s string) (TaintRule, error) {
	rule := TaintRule{}

	// Check for effect (last :Effect part)
	colonIdx := strings.LastIndex(s, ":")
	if colonIdx != -1 {
		effectStr := s[colonIdx+1:]
		keyValue := s[:colonIdx]

		effect, err := parseEffect(effectStr)
		if err != nil {
			// No valid effect found, treat entire string as key
			rule.Key = s
		} else {
			rule.Effect = &effect
			s = keyValue
		}
	}

	// Check for value (key=value part)
	if rule.Key == "" {
		if key, value, found := strings.Cut(s, "="); found {
			rule.Key = key
			rule.Value = &value
		} else {
			rule.Key = s
		}
	}

	if rule.Key == "" {
		return TaintRule{}, fmt.Errorf("empty key")
	}

	return rule, nil
}

// parseEffect parses a taint effect string
func parseEffect(s string) (corev1.TaintEffect, error) {
	switch s {
	case string(corev1.TaintEffectNoSchedule):
		return corev1.TaintEffectNoSchedule, nil
	case string(corev1.TaintEffectPreferNoSchedule):
		return corev1.TaintEffectPreferNoSchedule, nil
	case string(corev1.TaintEffectNoExecute):
		return corev1.TaintEffectNoExecute, nil
	default:
		return "", fmt.Errorf("invalid effect %q", s)
	}
}

// ShouldRemove checks if a taint matches any rule
func (r *TaintRemover) ShouldRemove(taint corev1.Taint) bool {
	for _, rule := range r.rules {
		if rule.Matches(taint) {
			return true
		}
	}
	return false
}

// Matches checks if a taint matches this rule
func (rule *TaintRule) Matches(taint corev1.Taint) bool {
	// Key must match
	if rule.Key != taint.Key {
		return false
	}

	// Value must match if specified
	if rule.Value != nil && *rule.Value != taint.Value {
		return false
	}

	// Effect must match if specified
	if rule.Effect != nil && *rule.Effect != taint.Effect {
		return false
	}

	return true
}

// GetTaintsToRemove returns taints that should be removed from a node
func (r *TaintRemover) GetTaintsToRemove(node *corev1.Node) []corev1.Taint {
	if node == nil || len(node.Spec.Taints) == 0 {
		return nil
	}

	var toRemove []corev1.Taint
	for _, taint := range node.Spec.Taints {
		if r.ShouldRemove(taint) {
			toRemove = append(toRemove, taint)
		}
	}
	return toRemove
}

// FilterOutTaints returns a new slice with the specified taints removed
func FilterOutTaints(taints []corev1.Taint, toRemove []corev1.Taint) []corev1.Taint {
	if len(toRemove) == 0 {
		return taints
	}

	removeSet := make(map[string]struct{})
	for _, t := range toRemove {
		removeSet[taintKey(t)] = struct{}{}
	}

	var result []corev1.Taint
	for _, t := range taints {
		if _, found := removeSet[taintKey(t)]; !found {
			result = append(result, t)
		}
	}
	return result
}

// taintKey returns a unique key for a taint
func taintKey(t corev1.Taint) string {
	return fmt.Sprintf("%s=%s:%s", t.Key, t.Value, t.Effect)
}

// TaintKeys returns a slice of taint keys for logging
func TaintKeys(taints []corev1.Taint) []string {
	keys := make([]string, len(taints))
	for i, t := range taints {
		keys[i] = taintKey(t)
	}
	return keys
}
