package cli

import "testing"

func TestDoctorCmd(t *testing.T) {
	cmd := doctorCmd()
	if cmd.Name != "doctor" {
		t.Errorf("expected command name %q, got %q", "doctor", cmd.Name)
	}
	if cmd.Action == nil {
		t.Error("expected non-nil action")
	}
}

func TestParseGitVersion(t *testing.T) {
	tests := []struct {
		input               string
		major, minor, patch int
		wantErr             bool
	}{
		{"git version 2.45.0", 2, 45, 0, false},
		{"git version 2.30.1", 2, 30, 1, false},
		{"git version 1.8.5", 1, 8, 5, false},
		{"git version 2.39.3 (Apple Git-146)", 2, 39, 3, false},
		{"git version 2.43.0.windows.1", 2, 43, 0, false},
		{"2.45.0", 2, 45, 0, false},
		{"", 0, 0, 0, true},
		{"git version abc.def.ghi", 0, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			major, minor, patch, err := parseGitVersion(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if major != tt.major || minor != tt.minor || patch != tt.patch {
				t.Errorf("got %d.%d.%d, want %d.%d.%d", major, minor, patch, tt.major, tt.minor, tt.patch)
			}
		})
	}
}
