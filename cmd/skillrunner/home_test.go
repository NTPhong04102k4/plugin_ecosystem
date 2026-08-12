package main

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestShimFilesPerPlatform pins the one platform difference that matters:
// Windows needs BOTH an extensionless script (Git Bash ignores PATHEXT) and a
// .cmd (cmd.exe and PowerShell need it). Shipping only one leaves `sr` broken in
// half the shells on this machine.
func TestShimFilesPerPlatform(t *testing.T) {
	if runtime.GOOS == "windows" {
		files := shimFiles(`C:\Users\x\go\bin\skillrunner.exe`)
		if len(files) != 2 {
			t.Fatalf("windows needs 2 shims, got %d: %v", len(files), keys(files))
		}
		cmd, ok := files["sr.cmd"]
		if !ok {
			t.Fatal("missing sr.cmd for cmd.exe/PowerShell")
		}
		if !strings.Contains(cmd, `%*`) {
			t.Errorf("sr.cmd must forward all args with %%*: %q", cmd)
		}
		sh, ok := files["sr"]
		if !ok {
			t.Fatal("missing extensionless sr for Git Bash")
		}
		// A POSIX shell treats \ as an escape, so the Windows path must be
		// converted before it lands in the script.
		if strings.Contains(sh, `\`) {
			t.Errorf("sh shim must not contain backslashes: %q", sh)
		}
		if !strings.Contains(sh, "C:/Users/x/go/bin/skillrunner.exe") {
			t.Errorf("sh shim should call the binary by forward-slash path: %q", sh)
		}
		return
	}

	files := shimFiles("/usr/local/bin/skillrunner")
	if len(files) != 1 {
		t.Fatalf("posix needs 1 shim, got %d: %v", len(files), keys(files))
	}
	sh, ok := files["sr"]
	if !ok {
		t.Fatal("missing sr")
	}
	if !strings.HasPrefix(sh, "#!/bin/sh\n") {
		t.Errorf("sh shim needs a shebang: %q", sh)
	}
	if !strings.Contains(sh, `"$@"`) {
		t.Errorf(`sh shim must forward args with "$@": %q`, sh)
	}
}

// TestShimForwardsQuotedArgs guards the quoting rule in both shim flavours: a
// pool path with a space must survive the hop, which is why the exe is quoted.
func TestShimForwardsQuotedArgs(t *testing.T) {
	for name, body := range shimFiles(filepath.Join("dir with space", "skillrunner")) {
		if !strings.Contains(body, `"`) {
			t.Errorf("shim %s does not quote the executable path: %q", name, body)
		}
	}
}

// TestWritePointerRejectsDirWithoutManifest: recording a path that resolves to
// nothing would recreate the exact failure this ladder exists to end — a setup
// that looks configured but is not.
func TestWritePointerRejectsDirWithoutManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	err := writePointer(t.TempDir()) // empty dir: no skill.json
	if err == nil {
		t.Fatal("want an error when --set points at a dir with no manifest")
	}
	if !strings.Contains(err.Error(), manifestName) {
		t.Errorf("error should name %s, got: %v", manifestName, err)
	}
	if p, perr := pointerPath(); perr == nil && fileExists(p) {
		t.Error("a rejected --set must not leave a pointer file behind")
	}
}

// TestWritePointerRoundTrip: what --set writes is what the ladder reads back.
func TestWritePointerRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(homeEnv, "")

	pool := t.TempDir()
	want := writeManifest(t, pool)

	if err := writePointer(pool); err != nil {
		t.Fatal(err)
	}
	got, via, err := resolveManifestPath("", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	if via != viaPointer {
		t.Errorf("via = %q, want %q", via, viaPointer)
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
