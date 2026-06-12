package agent

import (
	"slices"
	"testing"
)

func TestCommandResourceIDs_dedupesPayloadFormsPreservingFirstSeenOrder(t *testing.T) {
	for name, raw := range map[string]interface{}{
		"string slice":            []string{"resource_b", "resource_a", "resource_b", "resource_missing"},
		"decoded interface slice": []interface{}{"resource_b", "resource_a", "resource_b", "resource_missing"},
	} {
		t.Run(name, func(t *testing.T) {
			got := commandResourceIDs(map[string]interface{}{"resourceIds": raw})
			if !slices.Equal(got, []string{"resource_b", "resource_a", "resource_missing"}) {
				t.Fatalf("resource IDs = %v, want first-seen deduped order", got)
			}
		})
	}
}
