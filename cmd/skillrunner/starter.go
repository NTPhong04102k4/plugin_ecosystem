package main

// starterManifest is written by `skillrunner init`. It is a minimal but
// structurally-correct template: base rules + packRules + one core skill and one
// stack-parametrized skill. Add packs/<stack>.json to supply the stack content,
// and copy more skills from the ecosystem's skill.json as needed.
const starterManifest = `{
  "version": "1",
  "project": "my-project",

  // Base rules apply everywhere. Stack-specific content lives in packs/<stack>.json
  // and is merged in by "skillrunner emit" after detecting the stack.
  "rules": {
    "working-policy": [
      "When the request is ambiguous, ask before coding — do not guess.",
      "Write a short plan before any non-trivial code change.",
      "Minimal change only; stay within the requested scope.",
      "Verify with git status / git diff before finishing.",
      "Never commit on your own — present changes and let the user decide."
    ],
    "technical": [
      "Generated output must be deterministic: same input => same output.",
      "Never overwrite a file the tool did not generate without approval."
    ]
  },

  // Rule groups supplied by packs/<stack>.json (architecture, conventions, ...).
  "packRules": ["architecture", "conventions", "lint", "templates", "design-system", "library-docs"],

  "skills": {
    "plan-feature": {
      "description": "Turn a feature request into a plan + goals for the user to decide.",
      "goal": "Produce a reviewable plan; do NOT write code yet.",
      "appliesRules": ["working-policy"],
      "requiresApproval": true,
      "instructions": [
        "Restate the request in one sentence.",
        "List 2-4 measurable candidate goals for the user to pick.",
        "Draft a step-by-step plan naming the files that would change.",
        "Present everything and STOP for the user's decision."
      ],
      "outputs": ["A plan document — no code changes"]
    },

    "scaffold-data": {
      "description": "Build the data layer for an API call following the stack's architecture.",
      "goal": "Types/models + service/provider + wiring, per the pack's templates.",
      "appliesRules": ["working-policy", "technical", "architecture", "conventions", "templates"],
      "instructions": [
        "Confirm the endpoint contract; ask for missing request/response shapes.",
        "Copy the pack's data-layer template (do not invent a new structure).",
        "Generate the model/types (null-defensive) and the service/provider.",
        "Wire it into the repository/hook layer; do NOT build UI here."
      ],
      "outputs": ["Data-layer files following the pack conventions"]
    }
  }
}
`
