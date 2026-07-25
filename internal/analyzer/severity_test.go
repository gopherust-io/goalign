package analyzer

import "testing"

func TestGetSeverity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		wasted int
		want   string
	}{
		{0, "info"},
		{1, "low"},
		{7, "low"},
		{8, "medium"},
		{15, "medium"},
		{16, "high"},
		{100, "high"},
	}
	for _, tt := range tests {
		if got := getSeverity(tt.wasted); got != tt.want {
			t.Errorf("getSeverity(%d) = %q, want %q", tt.wasted, got, tt.want)
		}
	}
}
