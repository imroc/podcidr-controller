package selector

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func newNode(labels map[string]string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-node",
			Labels: labels,
		},
	}
}

func TestMatchesEmptySelector(t *testing.T) {
	s, _ := Parse("")
	node := newNode(map[string]string{"foo": "bar"})
	if !s.Matches(node) {
		t.Error("empty selector should match all nodes")
	}
}

func TestMatchesInOperator(t *testing.T) {
	s, _ := Parse(`[{"key":"node-type","operator":"In","values":["external","edge"]}]`)

	// Should match
	node1 := newNode(map[string]string{"node-type": "external"})
	if !s.Matches(node1) {
		t.Error("expected node with 'external' to match")
	}

	// Should match
	node2 := newNode(map[string]string{"node-type": "edge"})
	if !s.Matches(node2) {
		t.Error("expected node with 'edge' to match")
	}

	// Should not match - different value
	node3 := newNode(map[string]string{"node-type": "internal"})
	if s.Matches(node3) {
		t.Error("expected node with 'internal' to not match")
	}

	// Should not match - label missing
	node4 := newNode(map[string]string{"other": "label"})
	if s.Matches(node4) {
		t.Error("expected node without label to not match")
	}
}

func TestMatchesNotInOperator(t *testing.T) {
	s, _ := Parse(`[{"key":"zone","operator":"NotIn","values":["zone-a","zone-b"]}]`)

	// Should match - different value
	node1 := newNode(map[string]string{"zone": "zone-c"})
	if !s.Matches(node1) {
		t.Error("expected node with 'zone-c' to match NotIn")
	}

	// Should match - label missing
	node2 := newNode(map[string]string{"other": "label"})
	if !s.Matches(node2) {
		t.Error("expected node without zone label to match NotIn")
	}

	// Should not match
	node3 := newNode(map[string]string{"zone": "zone-a"})
	if s.Matches(node3) {
		t.Error("expected node with 'zone-a' to not match NotIn")
	}
}

func TestMatchesExistsOperator(t *testing.T) {
	s, _ := Parse(`[{"key":"node-type","operator":"Exists"}]`)

	// Should match - label exists
	node1 := newNode(map[string]string{"node-type": "anything"})
	if !s.Matches(node1) {
		t.Error("expected node with label to match Exists")
	}

	// Should not match - label missing
	node2 := newNode(map[string]string{"other": "label"})
	if s.Matches(node2) {
		t.Error("expected node without label to not match Exists")
	}
}

func TestMatchesDoesNotExistOperator(t *testing.T) {
	s, _ := Parse(`[{"key":"vpc-cni","operator":"DoesNotExist"}]`)

	// Should match - label missing
	node1 := newNode(map[string]string{"other": "label"})
	if !s.Matches(node1) {
		t.Error("expected node without label to match DoesNotExist")
	}

	// Should not match - label exists
	node2 := newNode(map[string]string{"vpc-cni": "true"})
	if s.Matches(node2) {
		t.Error("expected node with label to not match DoesNotExist")
	}
}

func TestMatchesGtOperator(t *testing.T) {
	s, _ := Parse(`[{"key":"priority","operator":"Gt","values":["5"]}]`)

	// Should match - 10 > 5
	node1 := newNode(map[string]string{"priority": "10"})
	if !s.Matches(node1) {
		t.Error("expected node with priority=10 to match Gt 5")
	}

	// Should not match - 5 not > 5
	node2 := newNode(map[string]string{"priority": "5"})
	if s.Matches(node2) {
		t.Error("expected node with priority=5 to not match Gt 5")
	}

	// Should not match - 3 < 5
	node3 := newNode(map[string]string{"priority": "3"})
	if s.Matches(node3) {
		t.Error("expected node with priority=3 to not match Gt 5")
	}

	// Should not match - label missing
	node4 := newNode(map[string]string{})
	if s.Matches(node4) {
		t.Error("expected node without label to not match Gt")
	}

	// Should not match - non-numeric value
	node5 := newNode(map[string]string{"priority": "high"})
	if s.Matches(node5) {
		t.Error("expected node with non-numeric value to not match Gt")
	}
}

func TestMatchesLtOperator(t *testing.T) {
	s, _ := Parse(`[{"key":"priority","operator":"Lt","values":["5"]}]`)

	// Should match - 3 < 5
	node1 := newNode(map[string]string{"priority": "3"})
	if !s.Matches(node1) {
		t.Error("expected node with priority=3 to match Lt 5")
	}

	// Should not match - 5 not < 5
	node2 := newNode(map[string]string{"priority": "5"})
	if s.Matches(node2) {
		t.Error("expected node with priority=5 to not match Lt 5")
	}

	// Should not match - 10 > 5
	node3 := newNode(map[string]string{"priority": "10"})
	if s.Matches(node3) {
		t.Error("expected node with priority=10 to not match Lt 5")
	}
}

func TestMatchesMultipleExpressions(t *testing.T) {
	// node-type=edge AND zone!=zone-a
	s, _ := Parse(`[{"key":"node-type","operator":"In","values":["edge"]},{"key":"zone","operator":"NotIn","values":["zone-a"]}]`)

	// Should match - both conditions met
	node1 := newNode(map[string]string{"node-type": "edge", "zone": "zone-b"})
	if !s.Matches(node1) {
		t.Error("expected node matching both conditions to match")
	}

	// Should not match - first condition fails
	node2 := newNode(map[string]string{"node-type": "internal", "zone": "zone-b"})
	if s.Matches(node2) {
		t.Error("expected node failing first condition to not match")
	}

	// Should not match - second condition fails
	node3 := newNode(map[string]string{"node-type": "edge", "zone": "zone-a"})
	if s.Matches(node3) {
		t.Error("expected node failing second condition to not match")
	}
}
