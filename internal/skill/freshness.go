package skill

import (
	"context"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
)

// Freshness reports how far a cached doc (docs/project-profile.md,
// docs/module-registry.md) has drifted from the working tree since it was last
// written.
//
// The doc is a committed file, so git already knows when it last changed — we
// need no marker inside the doc and no side-car state file. Everything here is
// derived from three cheap git queries against the commit that last touched it.
type Freshness struct {
	Known   bool // false when git is unavailable, dir is not a repo, or the doc is not committed
	Commits int  // commits between the doc's last change and HEAD
	Files   int  // source files changed over that span (docs excluded)
	Dirty   int  // source files currently modified or untracked
}

// Stale reports whether the doc has fallen behind the source it describes. Any
// source file changed since the doc was written counts: a profile that predates
// even one refactor can already name a symbol that no longer exists.
func (f Freshness) Stale() bool {
	return f.Known && (f.Files > 0 || f.Dirty > 0)
}

// gitTimeout bounds every git call. `status` is run at the head of a session and
// must stay instant — if git is slow (a huge repo, a stale index lock, a network
// filesystem), we report "unknown" rather than making the user wait.
const gitTimeout = 2 * time.Second

// CheckFreshness measures the drift of the doc at rel (slash-separated, relative
// to dir) against dir's git history.
//
// buildCommit, when non-empty, is HEAD as it stood when the doc was actually
// built (see Ledger.BuildCommit). It exists because the doc's own last commit is
// an optimistic baseline: fixing a typo in the doc resets it, and the doc then
// reads fresh while its content is still old. Both candidates are measured and
// the WORSE one wins, so the signal never claims fresher than the evidence
// supports — and comparing drift (rather than commit order) needs no merge-base
// and behaves sanely when the two commits sit on different branches.
//
// Every failure mode — git missing, dir not a repo, doc never committed, git
// slow — collapses to a zero Freshness with Known false. Staleness is advisory:
// not being able to compute it must never break `status`.
func CheckFreshness(dir, rel, buildCommit string) Freshness {
	last, ok := gitOut(dir, "log", "-1", "--format=%H", "--", rel)
	if !ok || last == "" {
		// No commit touches rel: either dir is not a repo, or the doc was written
		// but never committed. Both mean there is no baseline to measure from.
		return Freshness{}
	}

	f := driftSince(dir, last)
	if buildCommit != "" && buildCommit != last {
		if b := driftSince(dir, buildCommit); b.Known && b.worseThan(f) {
			f = b
		}
	}
	if out, ok := gitOut(dir, "status", "--porcelain"); ok {
		f.Dirty = countSourcePaths(porcelainPaths(out))
	}
	return f
}

// driftSince measures how far HEAD has moved past base. Dirty is left to the
// caller: it describes the working tree, not the span, so it is the same
// whichever baseline wins.
func driftSince(dir, base string) Freshness {
	n, ok := gitOut(dir, "rev-list", "--count", base+"..HEAD")
	if !ok {
		// base is not a commit this repo can resolve (rewritten history, a
		// ledger copied in from elsewhere) — no usable measurement.
		return Freshness{}
	}

	f := Freshness{Known: true}
	if c, err := strconv.Atoi(n); err == nil {
		f.Commits = c
	}
	if out, ok := gitOut(dir, "diff", "--name-only", base+"..HEAD"); ok {
		f.Files = countSourcePaths(strings.Split(out, "\n"))
	}
	return f
}

// worseThan reports whether f describes more drift than other, ranking by
// changed source files first — that is what actually invalidates a doc — and
// falling back to commit count when the file counts tie.
func (f Freshness) worseThan(other Freshness) bool {
	if f.Files != other.Files {
		return f.Files > other.Files
	}
	return f.Commits > other.Commits
}

// gitOut runs one git command in dir and returns its trimmed stdout. The bool is
// false for any failure; callers treat that as "unknown", never as an error.
func gitOut(dir string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// porcelainPaths pulls the path out of each `git status --porcelain` line. The
// format is "XY <path>", and renames appear as "R  <old> -> <new>" — we keep the
// destination, since that is the file that now exists.
func porcelainPaths(out string) []string {
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		p := strings.TrimSpace(line[3:])
		if _, dst, found := strings.Cut(p, " -> "); found {
			p = dst
		}
		paths = append(paths, strings.Trim(p, `"`))
	}
	return paths
}

// countSourcePaths counts the paths that describe real source, i.e. the material
// a cached doc summarizes.
func countSourcePaths(paths []string) int {
	n := 0
	for _, p := range paths {
		if isSourcePath(p) {
			n++
		}
	}
	return n
}

// isSourcePath reports whether a repo-relative path counts as source for
// staleness purposes.
//
// Prose and generated bookkeeping are excluded on purpose: editing docs/ or
// bumping the ledger must not mark the profile stale, or every `emit` would
// invalidate it and the signal would become noise people learn to ignore.
func isSourcePath(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}
	if strings.EqualFold(path.Ext(p), ".md") {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "docs", ".skillrunner", ".git":
			return false
		}
	}
	return true
}
