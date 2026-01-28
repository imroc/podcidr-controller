package selector

import (
	"encoding/json"
	"strconv"

	corev1 "k8s.io/api/core/v1"
)

// Expression represents a single label selector requirement
type Expression struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values,omitempty"`
}

// NodeSelector filters nodes based on label expressions
type NodeSelector struct {
	MatchExpressions []Expression
}

// Parse creates a NodeSelector from JSON string
// Empty string returns selector that matches all nodes
func Parse(jsonStr string) (*NodeSelector, error) {
	s := &NodeSelector{}
	if jsonStr == "" {
		return s, nil
	}

	var expressions []Expression
	if err := json.Unmarshal([]byte(jsonStr), &expressions); err != nil {
		return nil, err
	}
	s.MatchExpressions = expressions
	return s, nil
}

// Matches returns true if the node matches all expressions (AND logic)
// Empty selector matches all nodes
func (s *NodeSelector) Matches(node *corev1.Node) bool {
	if len(s.MatchExpressions) == 0 {
		return true
	}

	for _, expr := range s.MatchExpressions {
		if !matchExpression(node.Labels, expr) {
			return false
		}
	}
	return true
}

func matchExpression(labels map[string]string, expr Expression) bool {
	value, hasLabel := labels[expr.Key]

	switch expr.Operator {
	case "In":
		if !hasLabel {
			return false
		}
		return containsString(expr.Values, value)

	case "NotIn":
		if !hasLabel {
			return true
		}
		return !containsString(expr.Values, value)

	case "Exists":
		return hasLabel

	case "DoesNotExist":
		return !hasLabel

	case "Gt":
		if !hasLabel || len(expr.Values) == 0 {
			return false
		}
		labelVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return false
		}
		compareVal, err := strconv.ParseInt(expr.Values[0], 10, 64)
		if err != nil {
			return false
		}
		return labelVal > compareVal

	case "Lt":
		if !hasLabel || len(expr.Values) == 0 {
			return false
		}
		labelVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return false
		}
		compareVal, err := strconv.ParseInt(expr.Values[0], 10, 64)
		if err != nil {
			return false
		}
		return labelVal < compareVal

	default:
		return false
	}
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
