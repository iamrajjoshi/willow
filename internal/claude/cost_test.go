package claude

import (
	"testing"
	"time"
)

func TestEstimateFromSession(t *testing.T) {
	t.Run("nil session returns nil", func(t *testing.T) {
		got := EstimateFromSession(nil, 0, 0)
		if got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
	})

	t.Run("zero tool count gives base cost only", func(t *testing.T) {
		ss := &SessionStatus{
			Status:    StatusDone,
			SessionID: "abc",
			Timestamp: time.Now(),
		}
		got := EstimateFromSession(ss, 0, 0)
		if got == nil {
			t.Fatal("expected non-nil estimate")
		}
		if got.InputTokens != 5000 {
			t.Errorf("input tokens = %d, want 5000", got.InputTokens)
		}
		if got.OutputTokens != 0 {
			t.Errorf("output tokens = %d, want 0", got.OutputTokens)
		}
		if got.Method != "estimate" {
			t.Errorf("method = %q, want %q", got.Method, "estimate")
		}
	})

	t.Run("10 tool calls with 5 min duration", func(t *testing.T) {
		now := time.Now()
		ss := &SessionStatus{
			Status:    StatusDone,
			SessionID: "abc",
			ToolCount: 10,
			StartTime: now.Add(-5 * time.Minute),
			Timestamp: now,
		}
		got := EstimateFromSession(ss, 0, 0)
		if got == nil {
			t.Fatal("expected non-nil estimate")
		}
		// 5000 + 10*2000 + 5*500 = 5000 + 20000 + 2500 = 27500
		wantInput := int64(27500)
		if got.InputTokens != wantInput {
			t.Errorf("input tokens = %d, want %d", got.InputTokens, wantInput)
		}
		// 10 * 500 = 5000
		wantOutput := int64(5000)
		if got.OutputTokens != wantOutput {
			t.Errorf("output tokens = %d, want %d", got.OutputTokens, wantOutput)
		}
		// cost = 27500/1M * 3.0 + 5000/1M * 15.0 = 0.0825 + 0.075 = 0.1575
		wantUSD := 0.1575
		if got.TotalUSD < wantUSD-0.001 || got.TotalUSD > wantUSD+0.001 {
			t.Errorf("total USD = %f, want ~%f", got.TotalUSD, wantUSD)
		}
	})

	t.Run("custom rates override defaults", func(t *testing.T) {
		ss := &SessionStatus{
			Status:    StatusDone,
			SessionID: "abc",
			ToolCount: 10,
			Timestamp: time.Now(),
		}
		got := EstimateFromSession(ss, 15.0, 75.0)
		if got == nil {
			t.Fatal("expected non-nil estimate")
		}
		// input: 5000 + 10*2000 = 25000, output: 10*500 = 5000
		// cost = 25000/1M * 15.0 + 5000/1M * 75.0 = 0.375 + 0.375 = 0.75
		wantUSD := 0.75
		if got.TotalUSD < wantUSD-0.001 || got.TotalUSD > wantUSD+0.001 {
			t.Errorf("total USD = %f, want ~%f", got.TotalUSD, wantUSD)
		}
	})

	t.Run("zero rates use defaults", func(t *testing.T) {
		ss := &SessionStatus{
			Status:    StatusDone,
			SessionID: "abc",
			Timestamp: time.Now(),
		}
		gotZero := EstimateFromSession(ss, 0, 0)
		gotNeg := EstimateFromSession(ss, -1, -1)
		if gotZero.TotalUSD != gotNeg.TotalUSD {
			t.Errorf("zero rates (%f) != negative rates (%f), both should use defaults",
				gotZero.TotalUSD, gotNeg.TotalUSD)
		}
	})
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		name string
		est  *CostEstimate
		want string
	}{
		{"nil", nil, "--"},
		{"very small cost", &CostEstimate{TotalUSD: 0.001}, "~$0.00"},
		{"normal cost", &CostEstimate{TotalUSD: 0.42}, "~$0.42"},
		{"larger cost", &CostEstimate{TotalUSD: 1.23}, "~$1.23"},
		{"exactly zero", &CostEstimate{TotalUSD: 0.0}, "~$0.00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatCost(tt.est)
			if got != tt.want {
				t.Errorf("FormatCost() = %q, want %q", got, tt.want)
			}
		})
	}
}
