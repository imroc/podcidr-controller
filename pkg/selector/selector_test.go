package selector

import (
	"testing"
)

func TestParseEmpty(t *testing.T) {
	s, err := Parse("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.MatchExpressions) != 0 {
		t.Errorf("expected empty expressions, got %d", len(s.MatchExpressions))
	}
}

func TestParseValidJSON(t *testing.T) {
	input := `[{"key":"node-type","operator":"In","values":["external"]}]`
	s, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.MatchExpressions) != 1 {
		t.Fatalf("expected 1 expression, got %d", len(s.MatchExpressions))
	}
	if s.MatchExpressions[0].Key != "node-type" {
		t.Errorf("expected key 'node-type', got '%s'", s.MatchExpressions[0].Key)
	}
	if s.MatchExpressions[0].Operator != "In" {
		t.Errorf("expected operator 'In', got '%s'", s.MatchExpressions[0].Operator)
	}
}

func TestParseInvalidJSON(t *testing.T) {
	_, err := Parse("not valid json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
