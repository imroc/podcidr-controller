package selector

import (
	"encoding/json"
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
