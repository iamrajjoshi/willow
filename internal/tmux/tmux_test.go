package tmux

import (
	"testing"
)

func TestPrepareLayoutCmd_InjectsSessionTarget(t *testing.T) {
	args := prepareLayoutCmd([]string{"split-window", "-h"}, "mysession", "/tmp/dir")

	if args[0] != "split-window" {
		t.Errorf("args[0] = %q, want %q", args[0], "split-window")
	}
	if args[1] != "-t" || args[2] != "mysession" {
		t.Errorf("expected -t mysession, got %v", args[1:3])
	}
}

func TestPrepareLayoutCmd_InjectsWorkingDir(t *testing.T) {
	args := prepareLayoutCmd([]string{"split-window", "-h"}, "sess", "/work")

	found := false
	for i, a := range args {
		if a == "-c" && i+1 < len(args) && args[i+1] == "/work" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected -c /work in args, got %v", args)
	}
}

func TestPrepareLayoutCmd_SkipsTargetIfPresent(t *testing.T) {
	args := prepareLayoutCmd([]string{"split-window", "-t", "custom"}, "sess", "/work")

	count := 0
	for _, a := range args {
		if a == "-t" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 -t flag, got %d in %v", count, args)
	}
}

func TestPrepareLayoutCmd_SkipsDirIfPresent(t *testing.T) {
	args := prepareLayoutCmd([]string{"split-window", "-c", "/custom"}, "sess", "/work")

	count := 0
	for _, a := range args {
		if a == "-c" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 -c flag, got %d in %v", count, args)
	}
}

func TestPrepareLayoutCmd_NoDirForSelectLayout(t *testing.T) {
	args := prepareLayoutCmd([]string{"select-layout", "even-horizontal"}, "sess", "/work")

	for _, a := range args {
		if a == "-c" {
			t.Errorf("select-layout should not get -c flag, got %v", args)
		}
	}
}

func TestPrepareLayoutCmd_NewWindowGetsDir(t *testing.T) {
	args := prepareLayoutCmd([]string{"new-window"}, "sess", "/work")

	found := false
	for i, a := range args {
		if a == "-c" && i+1 < len(args) && args[i+1] == "/work" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("new-window should get -c /work, got %v", args)
	}
}

func TestPrepareLayoutCmd_EmptyArgs(t *testing.T) {
	args := prepareLayoutCmd([]string{}, "sess", "/work")
	if len(args) != 0 {
		t.Errorf("expected empty args, got %v", args)
	}
}
