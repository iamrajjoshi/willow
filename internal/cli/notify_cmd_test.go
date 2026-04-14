package cli

import "testing"

func TestNotifyCmd_Structure(t *testing.T) {
	cmd := notifyCmd()
	if cmd.Name != "notify" {
		t.Errorf("Name = %q, want %q", cmd.Name, "notify")
	}

	wantSubs := map[string]bool{"on": false, "off": false, "status": false, "run": false}
	for _, sub := range cmd.Commands {
		if _, ok := wantSubs[sub.Name]; ok {
			wantSubs[sub.Name] = true
		}
	}
	for name, found := range wantSubs {
		if !found {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestNotifyRunCmd_Hidden(t *testing.T) {
	cmd := notifyCmd()
	for _, sub := range cmd.Commands {
		if sub.Name == "run" && !sub.Hidden {
			t.Error("'run' subcommand should be hidden")
		}
	}
}
