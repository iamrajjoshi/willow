package cli

import "testing"

func TestStackCmd(t *testing.T) {
	cmd := stackCmd()
	if cmd.Name != "stack" {
		t.Errorf("Name = %q, want %q", cmd.Name, "stack")
	}
	if len(cmd.Commands) == 0 {
		t.Fatal("expected subcommands, got none")
	}

	var found bool
	for _, sub := range cmd.Commands {
		if sub.Name == "status" {
			found = true
			if len(sub.Aliases) == 0 || sub.Aliases[0] != "s" {
				t.Errorf("status aliases = %v, want [s]", sub.Aliases)
			}
			break
		}
	}
	if !found {
		t.Error("expected 'status' subcommand")
	}
}
