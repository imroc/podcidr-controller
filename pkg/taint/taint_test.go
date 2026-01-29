package taint

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewTaintRemover(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		wantNil     bool
		wantErr     bool
		wantRules   int
	}{
		{
			name:    "empty config returns nil",
			config:  "",
			wantNil: true,
		},
		{
			name:    "whitespace only returns nil",
			config:  "   ",
			wantNil: true,
		},
		{
			name:      "single key only",
			config:    "node.kubernetes.io/not-ready",
			wantRules: 1,
		},
		{
			name:      "key with effect",
			config:    "node.kubernetes.io/not-ready:NoSchedule",
			wantRules: 1,
		},
		{
			name:      "key with value and effect",
			config:    "dedicated=gpu:NoSchedule",
			wantRules: 1,
		},
		{
			name:      "multiple rules",
			config:    "key1,key2:NoSchedule,key3=value:NoExecute",
			wantRules: 3,
		},
		{
			name:      "rules with whitespace",
			config:    " key1 , key2:NoSchedule , key3=value:NoExecute ",
			wantRules: 3,
		},
		{
			name:      "tke eni unavailable taint",
			config:    "tke.cloud.tencent.com/eni-ip-unavailable",
			wantRules: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewTaintRemover(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTaintRemover() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantNil {
				if got != nil {
					t.Errorf("NewTaintRemover() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Errorf("NewTaintRemover() = nil, want non-nil")
				return
			}
			if len(got.rules) != tt.wantRules {
				t.Errorf("NewTaintRemover() rules count = %d, want %d", len(got.rules), tt.wantRules)
			}
		})
	}
}

func TestParseRule(t *testing.T) {
	noSchedule := corev1.TaintEffectNoSchedule
	noExecute := corev1.TaintEffectNoExecute
	preferNoSchedule := corev1.TaintEffectPreferNoSchedule

	tests := []struct {
		name       string
		input      string
		wantKey    string
		wantValue  *string
		wantEffect *corev1.TaintEffect
		wantErr    bool
	}{
		{
			name:    "key only",
			input:   "node.kubernetes.io/not-ready",
			wantKey: "node.kubernetes.io/not-ready",
		},
		{
			name:       "key with NoSchedule",
			input:      "node.kubernetes.io/not-ready:NoSchedule",
			wantKey:    "node.kubernetes.io/not-ready",
			wantEffect: &noSchedule,
		},
		{
			name:       "key with NoExecute",
			input:      "node.kubernetes.io/unreachable:NoExecute",
			wantKey:    "node.kubernetes.io/unreachable",
			wantEffect: &noExecute,
		},
		{
			name:       "key with PreferNoSchedule",
			input:      "node.kubernetes.io/memory-pressure:PreferNoSchedule",
			wantKey:    "node.kubernetes.io/memory-pressure",
			wantEffect: &preferNoSchedule,
		},
		{
			name:       "key=value:effect",
			input:      "dedicated=gpu:NoSchedule",
			wantKey:    "dedicated",
			wantValue:  strPtr("gpu"),
			wantEffect: &noSchedule,
		},
		{
			name:      "key=value without effect",
			input:     "dedicated=gpu",
			wantKey:   "dedicated",
			wantValue: strPtr("gpu"),
		},
		{
			name:    "key with invalid effect treated as key",
			input:   "node.kubernetes.io/not-ready:InvalidEffect",
			wantKey: "node.kubernetes.io/not-ready:InvalidEffect",
		},
		{
			name:       "empty value with effect",
			input:      "key=:NoSchedule",
			wantKey:    "key",
			wantValue:  strPtr(""),
			wantEffect: &noSchedule,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRule(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseRule() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got.Key != tt.wantKey {
				t.Errorf("parseRule() Key = %q, want %q", got.Key, tt.wantKey)
			}
			if !strPtrEqual(got.Value, tt.wantValue) {
				t.Errorf("parseRule() Value = %v, want %v", strPtrStr(got.Value), strPtrStr(tt.wantValue))
			}
			if !effectPtrEqual(got.Effect, tt.wantEffect) {
				t.Errorf("parseRule() Effect = %v, want %v", effectPtrStr(got.Effect), effectPtrStr(tt.wantEffect))
			}
		})
	}
}

func TestTaintRemover_ShouldRemove(t *testing.T) {
	remover, err := NewTaintRemover("key1,key2:NoSchedule,key3=value3:NoExecute")
	if err != nil {
		t.Fatalf("NewTaintRemover() error = %v", err)
	}

	tests := []struct {
		name  string
		taint corev1.Taint
		want  bool
	}{
		{
			name:  "matches key only rule",
			taint: corev1.Taint{Key: "key1", Value: "any", Effect: corev1.TaintEffectNoSchedule},
			want:  true,
		},
		{
			name:  "matches key only rule with different effect",
			taint: corev1.Taint{Key: "key1", Value: "", Effect: corev1.TaintEffectNoExecute},
			want:  true,
		},
		{
			name:  "matches key:effect rule",
			taint: corev1.Taint{Key: "key2", Value: "any", Effect: corev1.TaintEffectNoSchedule},
			want:  true,
		},
		{
			name:  "key:effect rule - wrong effect",
			taint: corev1.Taint{Key: "key2", Value: "any", Effect: corev1.TaintEffectNoExecute},
			want:  false,
		},
		{
			name:  "matches key=value:effect rule",
			taint: corev1.Taint{Key: "key3", Value: "value3", Effect: corev1.TaintEffectNoExecute},
			want:  true,
		},
		{
			name:  "key=value:effect rule - wrong value",
			taint: corev1.Taint{Key: "key3", Value: "wrong", Effect: corev1.TaintEffectNoExecute},
			want:  false,
		},
		{
			name:  "key=value:effect rule - wrong effect",
			taint: corev1.Taint{Key: "key3", Value: "value3", Effect: corev1.TaintEffectNoSchedule},
			want:  false,
		},
		{
			name:  "no match",
			taint: corev1.Taint{Key: "unknown", Value: "", Effect: corev1.TaintEffectNoSchedule},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := remover.ShouldRemove(tt.taint); got != tt.want {
				t.Errorf("ShouldRemove() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaintRemover_GetTaintsToRemove(t *testing.T) {
	remover, err := NewTaintRemover("tke.cloud.tencent.com/eni-ip-unavailable,node.kubernetes.io/not-ready:NoSchedule")
	if err != nil {
		t.Fatalf("NewTaintRemover() error = %v", err)
	}

	tests := []struct {
		name     string
		node     *corev1.Node
		wantKeys []string
	}{
		{
			name: "nil node",
			node: nil,
		},
		{
			name: "node with no taints",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
			},
		},
		{
			name: "node with matching taint",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{Key: "tke.cloud.tencent.com/eni-ip-unavailable", Effect: corev1.TaintEffectNoSchedule},
					},
				},
			},
			wantKeys: []string{"tke.cloud.tencent.com/eni-ip-unavailable"},
		},
		{
			name: "node with multiple matching taints",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{Key: "tke.cloud.tencent.com/eni-ip-unavailable", Effect: corev1.TaintEffectNoSchedule},
						{Key: "node.kubernetes.io/not-ready", Effect: corev1.TaintEffectNoSchedule},
						{Key: "other-taint", Effect: corev1.TaintEffectNoSchedule},
					},
				},
			},
			wantKeys: []string{"tke.cloud.tencent.com/eni-ip-unavailable", "node.kubernetes.io/not-ready"},
		},
		{
			name: "node with non-matching taints only",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{Key: "other-taint", Effect: corev1.TaintEffectNoSchedule},
					},
				},
			},
		},
		{
			name: "key:effect rule - effect mismatch",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{Key: "node.kubernetes.io/not-ready", Effect: corev1.TaintEffectNoExecute},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := remover.GetTaintsToRemove(tt.node)
			if len(got) != len(tt.wantKeys) {
				t.Errorf("GetTaintsToRemove() returned %d taints, want %d", len(got), len(tt.wantKeys))
				return
			}
			for i, taint := range got {
				if taint.Key != tt.wantKeys[i] {
					t.Errorf("GetTaintsToRemove()[%d].Key = %q, want %q", i, taint.Key, tt.wantKeys[i])
				}
			}
		})
	}
}

func TestFilterOutTaints(t *testing.T) {
	tests := []struct {
		name     string
		taints   []corev1.Taint
		toRemove []corev1.Taint
		wantKeys []string
	}{
		{
			name:     "empty toRemove",
			taints:   []corev1.Taint{{Key: "key1"}},
			toRemove: nil,
			wantKeys: []string{"key1"},
		},
		{
			name:     "remove single taint",
			taints:   []corev1.Taint{{Key: "key1"}, {Key: "key2"}},
			toRemove: []corev1.Taint{{Key: "key1"}},
			wantKeys: []string{"key2"},
		},
		{
			name:     "remove all taints",
			taints:   []corev1.Taint{{Key: "key1"}, {Key: "key2"}},
			toRemove: []corev1.Taint{{Key: "key1"}, {Key: "key2"}},
			wantKeys: nil,
		},
		{
			name:     "remove with value and effect match",
			taints:   []corev1.Taint{{Key: "key1", Value: "v1", Effect: corev1.TaintEffectNoSchedule}},
			toRemove: []corev1.Taint{{Key: "key1", Value: "v1", Effect: corev1.TaintEffectNoSchedule}},
			wantKeys: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterOutTaints(tt.taints, tt.toRemove)
			if len(got) != len(tt.wantKeys) {
				t.Errorf("FilterOutTaints() returned %d taints, want %d", len(got), len(tt.wantKeys))
				return
			}
			for i, taint := range got {
				if taint.Key != tt.wantKeys[i] {
					t.Errorf("FilterOutTaints()[%d].Key = %q, want %q", i, taint.Key, tt.wantKeys[i])
				}
			}
		})
	}
}

// Helper functions
func strPtr(s string) *string {
	return &s
}

func strPtrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func strPtrStr(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func effectPtrEqual(a, b *corev1.TaintEffect) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func effectPtrStr(p *corev1.TaintEffect) string {
	if p == nil {
		return "<nil>"
	}
	return string(*p)
}
