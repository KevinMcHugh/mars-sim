package sim

import "testing"

func TestLogKindLabelsAreDistinct(t *testing.T) {
	seen := map[string]LogKind{}
	for k := LogNote; k < LogKindCount; k++ {
		label := k.String()
		if label == "" {
			t.Errorf("kind %d has an empty label", k)
		}
		if prev, ok := seen[label]; ok {
			t.Errorf("kinds %d and %d both use %q", prev, k, label)
		}
		seen[label] = k
	}
}
