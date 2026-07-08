package skill

import (
	"fmt"
	"strings"
)

// Emit composes the full marching orders for a single skill: its goal, the
// shared rules it must obey, the ordered steps, expected outputs, and any
// approval gate. The output is Markdown meant to be read and executed by Claude
// — the binary itself does no reasoning here.
func (m *Manifest) Emit(name string) (string, error) {
	s, ok := m.Skills[name]
	if !ok {
		return "", fmt.Errorf("unknown skill %q (run `skillrunner list` to see available skills)", name)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# SKILL: %s\n\n", name)
	fmt.Fprintf(&b, "%s\n\n", s.Description)
	if s.Goal != "" {
		fmt.Fprintf(&b, "## Goal\n%s\n\n", s.Goal)
	}

	// Rules come before instructions so Claude reads the constraints first.
	if len(s.AppliesRules) > 0 {
		b.WriteString("## Rules you MUST follow\n")
		var missing []string
		for _, group := range s.AppliesRules {
			items := m.Rules[group]
			if len(items) == 0 {
				if m.isPackRule(group) {
					missing = append(missing, group)
				}
				continue
			}
			fmt.Fprintf(&b, "\n**%s**\n", capitalize(group))
			for _, r := range items {
				fmt.Fprintf(&b, "- %s\n", r)
			}
		}
		if len(missing) > 0 {
			fmt.Fprintf(&b, "\n> ⚠ Stack rule groups not loaded: %s. Run `skillrunner detect` and emit with a pack so these are filled in before proceeding.\n", strings.Join(missing, ", "))
		}
		b.WriteString("\n")
	}

	if len(s.Inputs) > 0 {
		b.WriteString("## Inputs / context to gather first\n")
		for _, in := range s.Inputs {
			fmt.Fprintf(&b, "- %s\n", in)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Steps\n")
	for i, step := range s.Instructions {
		fmt.Fprintf(&b, "%d. %s\n", i+1, step)
	}
	b.WriteString("\n")

	if len(s.Outputs) > 0 {
		b.WriteString("## Expected outputs\n")
		for _, o := range s.Outputs {
			fmt.Fprintf(&b, "- %s\n", o)
		}
		b.WriteString("\n")
	}

	if s.RequiresApproval {
		b.WriteString("## Approval gate\n")
		b.WriteString("This skill produces a PLAN/PROPOSAL only. Present the result to the user and STOP for their decision before writing or changing any project file.\n\n")
	}

	return b.String(), nil
}

// capitalize upper-cases the first letter of an ASCII rule-group name.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// List renders a one-line-per-skill summary for humans and for Claude to pick from.
func (m *Manifest) List() string {
	var b strings.Builder
	if m.Project != "" {
		fmt.Fprintf(&b, "Project: %s\n\n", m.Project)
	}
	b.WriteString("Available skills:\n")
	for _, name := range m.SkillNames() {
		s := m.Skills[name]
		gate := ""
		if s.RequiresApproval {
			gate = " [needs approval]"
		}
		fmt.Fprintf(&b, "  %-20s %s%s\n", name, s.Description, gate)
	}
	return b.String()
}
