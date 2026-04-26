package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

func TestCopyDirCopiesFilesDirectoriesAndSymlinks(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	dest := filepath.Join(t.TempDir(), "dest")
	nested := filepath.Join(source, "nested")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	filePath := filepath.Join(nested, "file.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o640); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.Symlink("nested/file.txt", filepath.Join(source, "link.txt")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := copyDir(source, dest); err != nil {
		t.Fatalf("copyDir: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dest, "nested", "file.txt"))
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("copied file = %q, want hello", data)
	}
	linkTarget, err := os.Readlink(filepath.Join(dest, "link.txt"))
	if err != nil {
		t.Fatalf("read copied symlink: %v", err)
	}
	if linkTarget != "nested/file.txt" {
		t.Fatalf("symlink target = %q, want nested/file.txt", linkTarget)
	}
	info, err := os.Stat(filepath.Join(dest, "nested"))
	if err != nil {
		t.Fatalf("stat copied dir: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o750 {
		t.Fatalf("copied dir mode = %o, want 750", got)
	}
}

func TestCopyDirRejectsNonDirectorySource(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	err := copyDir(source, filepath.Join(t.TempDir(), "dest"))
	if err == nil {
		t.Fatal("copyDir should reject non-directory source")
	}
	if !strings.Contains(err.Error(), "source is not a directory") {
		t.Fatalf("error = %v, want non-directory message", err)
	}
}

func TestCopyFileCopiesContentsAndMode(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.txt")
	dest := filepath.Join(t.TempDir(), "dest.txt")
	if err := os.WriteFile(source, []byte("copy me"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	if err := copyFile(source, dest, 0o640); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(data) != "copy me" {
		t.Fatalf("dest contents = %q, want copy me", data)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("dest mode = %o, want 640", got)
	}
}

func TestMoveWillowBaseCopiesWhenDestinationExists(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	dest := filepath.Join(t.TempDir(), "dest")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "state.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	if err := moveWillowBase(source, dest); err != nil {
		t.Fatalf("moveWillowBase: %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "state.json")); err != nil {
		t.Fatalf("dest file missing: %v", err)
	}
}

func TestIsCrossDeviceError(t *testing.T) {
	if !isCrossDeviceError(syscall.EXDEV) {
		t.Fatal("syscall.EXDEV should be cross-device")
	}
	if !isCrossDeviceError(&os.LinkError{Op: "rename", Old: "a", New: "b", Err: syscall.EXDEV}) {
		t.Fatal("LinkError wrapping EXDEV should be cross-device")
	}
	if isCrossDeviceError(errors.New("boom")) {
		t.Fatal("plain error should not be cross-device")
	}
}

func TestPrefixLines(t *testing.T) {
	got := prefixLines([]string{"one", "two"}, "  ")
	want := []string{"  one", "  two"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prefixLines() = %v, want %v", got, want)
	}
}
