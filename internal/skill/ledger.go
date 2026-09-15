package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ledgerDir is the per-project folder that holds skillrunner's local state, and
// ledgerFile is the applied-skills record inside it. The ledger lives in the
// TARGET project (not the central skill pool) so each app tracks its own history
// — which skill was emitted, how many times, and when. This is what lets a later
// session (or a forgetful human) see "we already ran build-ui here" instead of
// re-emitting from scratch.
const (
	ledgerDir  = ".skillrunner"
	ledgerFile = "ledger.json"
)

// Ledger is the applied-skills record for a single project directory.
type Ledger struct {
	Project   string                   `json:"project,omitempty"`
	UpdatedAt string                   `json:"updatedAt,omitempty"`
	Skills    map[string]*LedgerRecord `json:"skills"`
}

// LedgerRecord tracks one skill's emit history within a project.
//
// LastCommit is HEAD at the moment of the most recent emit. For a skill that
// builds a cached doc (learn-project, update-module-registry) that is the real
// "built at" marker, which the doc's own commit history cannot supply — see
// Ledger.BuildCommit.
type LedgerRecord struct {
	Stack        string `json:"stack,omitempty"`
	Count        int    `json:"count"`
	FirstEmitted string `json:"firstEmitted,omitempty"`
	LastEmitted  string `json:"lastEmitted,omitempty"`
	LastCommit   string `json:"lastCommit,omitempty"`
}

// ledgerPath returns the ledger file path for a project directory.
func ledgerPath(dir string) string {
	return filepath.Join(dir, ledgerDir, ledgerFile)
}

// LoadLedger reads the ledger for dir. A missing ledger is not an error — it
// returns an empty, ready-to-use Ledger so callers can treat "never recorded"
// and "recorded nothing" the same way.
func LoadLedger(dir string) (*Ledger, error) {
	l := &Ledger{Skills: map[string]*LedgerRecord{}}
	data, err := os.ReadFile(ledgerPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return l, nil
		}
		return nil, fmt.Errorf("read ledger: %w", err)
	}
	if err := json.Unmarshal(data, l); err != nil {
		return nil, fmt.Errorf("parse ledger %q: %w", ledgerPath(dir), err)
	}
	if l.Skills == nil {
		l.Skills = map[string]*LedgerRecord{}
	}
	return l, nil
}

// save writes the ledger to disk, creating the .skillrunner dir if needed.
func (l *Ledger) save(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, ledgerDir), 0o755); err != nil {
		return fmt.Errorf("create %s dir: %w", ledgerDir, err)
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ledger: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(ledgerPath(dir), data, 0o644); err != nil {
		return fmt.Errorf("write ledger: %w", err)
	}
	return nil
}

// RecordEmit upserts a skill's emit record in dir's ledger and saves it. now is
// injected so the caller controls the clock (and tests stay deterministic). The
// project label is stored for context in `sr ledger` output.
func RecordEmit(dir, project, name, stack string, now time.Time) error {
	l, err := LoadLedger(dir)
	if err != nil {
		return err
	}
	stamp := now.UTC().Format(time.RFC3339)
	if project != "" {
		l.Project = project
	}
	l.UpdatedAt = stamp

	rec := l.Skills[name]
	if rec == nil {
		rec = &LedgerRecord{FirstEmitted: stamp}
		l.Skills[name] = rec
	}
	rec.Count++
	rec.LastEmitted = stamp
	if stack != "" {
		rec.Stack = stack
	}
	// Best-effort: outside a repo (or without git) there is simply no commit to
	// record, and freshness falls back to the doc's own history.
	if head, ok := gitOut(dir, "rev-parse", "HEAD"); ok {
		rec.LastCommit = head
	}
	return l.save(dir)
}

// BuildCommit returns HEAD as it stood when skill was last emitted here, or ""
// if the skill was never emitted, predates commit recording, or ran outside a
// repo. Callers treat "" as "no better baseline than the file's own history".
func (l *Ledger) BuildCommit(skill string) string {
	rec := l.Skills[skill]
	if rec == nil {
		return ""
	}
	return rec.LastCommit
}

// names returns the recorded skill names sorted for stable output.
func (l *Ledger) names() []string {
	out := make([]string, 0, len(l.Skills))
	for n := range l.Skills {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Summary renders a one-line-per-skill list of what has been emitted here, for
// `sr ledger`. Returns a friendly note when nothing has been recorded yet.
func (l *Ledger) Summary() string {
	if len(l.Skills) == 0 {
		return "No skills emitted here yet (this ledger is empty).\n"
	}
	var b strings.Builder
	if l.Project != "" {
		fmt.Fprintf(&b, "Project: %s\n", l.Project)
	}
	b.WriteString("Skills already emitted here:\n")
	for _, name := range l.names() {
		r := l.Skills[name]
		stack := r.Stack
		if stack == "" {
			stack = "?"
		}
		fmt.Fprintf(&b, "  %-20s ×%-3d last %s  [%s]\n", name, r.Count, shortDate(r.LastEmitted), stack)
	}
	return b.String()
}

// StatusLine renders a compact one-liner of emitted skills for `sr status`.
func (l *Ledger) StatusLine() string {
	if len(l.Skills) == 0 {
		return "Emitted: none yet — no skills recorded in .skillrunner/ledger.json"
	}
	parts := make([]string, 0, len(l.Skills))
	for _, name := range l.names() {
		parts = append(parts, fmt.Sprintf("%s×%d", name, l.Skills[name].Count))
	}
	return "Emitted: " + strings.Join(parts, ", ") + " (see `sr ledger`)"
}

// shortDate trims an RFC3339 timestamp to its date for compact display.
func shortDate(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	if ts == "" {
		return "-"
	}
	return ts
}
