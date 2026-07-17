package skill

import (
	"os"
	"path/filepath"
	"strings"
)

// Detection is the result of inspecting a project directory.
type Detection struct {
	Stack  string // pack name, e.g. "react", "flutter", "" if unknown
	Reason string // which signal matched
}

// maxDetectDepth bounds how deep the recursive walk descends below root.
// 0 = root only, 1 = root + immediate children, etc. Two levels covers the
// common monorepo shapes (apps/web, frontend/, packages/foo) without turning
// detection into a full-tree crawl.
const maxDetectDepth = 2

// skipDirs are directories that never hold a project's real stack signature and
// would only add noise (or false positives) to the walk.
var skipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	".dart_tool":   true,
	"build":        true,
	"dist":         true,
	"out":          true,
	"vendor":       true,
	"Pods":         true,
	".gradle":      true,
	".idea":        true,
	".vscode":      true,
	"bin":          true,
	"obj":          true,
	".next":        true,
	".expo":        true,
	"target":       true,
}

// Detect inspects root (and, if nothing matches at the top, its subdirectories
// up to maxDetectDepth) for signature files and returns the stack (pack name).
// This is pure, deterministic file inspection — no reasoning. The walk is
// breadth-first, so a shallower match always wins: a signature at the root beats
// one nested in a subdirectory, which handles monorepos where the real project
// lives in apps/web, frontend/, etc. Within a single directory the more specific
// signal (react-native before react) is checked first. Returns an empty Stack if
// nothing matches anywhere in range.
func Detect(root string) Detection {
	// BFS queue of (dir, depth). Root first, then each level outward.
	type node struct {
		dir   string
		depth int
	}
	queue := []node{{root, 0}}

	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]

		if d := detectDir(n.dir, root); d.Stack != "" {
			return d
		}

		if n.depth >= maxDetectDepth {
			continue
		}
		entries, err := os.ReadDir(n.dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if skipDirs[name] || strings.HasPrefix(name, ".") {
				continue
			}
			queue = append(queue, node{filepath.Join(n.dir, name), n.depth + 1})
		}
	}

	return Detection{"", "no known stack signature found (searched " + rel(root) + " and up to " + depthWord() + " levels of subdirectories)"}
}

// detectDir runs the single-directory signature checks against dir. root is the
// original detection root, used only to render a friendly "where" in the reason
// when the match is in a subdirectory.
func detectDir(dir, root string) Detection {
	has := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}
	glob := func(pattern string) bool {
		m, _ := filepath.Glob(filepath.Join(dir, pattern))
		return len(m) > 0
	}
	// where annotates the reason with the subdirectory, if any, so `detect`
	// output tells the user exactly which folder triggered the match.
	where := func(msg string) string {
		if r, err := filepath.Rel(root, dir); err == nil && r != "." {
			return msg + " in ./" + filepath.ToSlash(r)
		}
		return msg
	}

	// Flutter: pubspec.yaml is unambiguous.
	if has("pubspec.yaml") {
		return Detection{"flutter", where("pubspec.yaml present")}
	}

	// JS ecosystem: distinguish React Native from React by package.json contents.
	if has("package.json") {
		pkg, _ := os.ReadFile(filepath.Join(dir, "package.json"))
		s := string(pkg)
		switch {
		case strings.Contains(s, "\"react-native\""):
			return Detection{"rn", where("react-native in package.json")}
		case strings.Contains(s, "\"react\""):
			return Detection{"react", where("react in package.json")}
		}
	}

	// Android native (Kotlin or Java). The Kotlin DSL (.kts) is a strong Kotlin
	// signal; a plain Groovy Gradle build could be either, so peek the build
	// scripts for the Kotlin plugin. No signal => treat it as a Java project.
	if has("build.gradle.kts") || has("settings.gradle.kts") {
		return Detection{"kotlin", where("Kotlin-DSL gradle files present")}
	}
	if has("build.gradle") || has("settings.gradle") {
		if gradleUsesKotlin(dir) {
			return Detection{"kotlin", where("gradle build with Kotlin plugin")}
		}
		return Detection{"java", where("gradle build, no Kotlin signal")}
	}

	// iOS native (Swift).
	if has("Package.swift") || glob("*.xcodeproj") || glob("*.xcworkspace") {
		return Detection{"swift", where("Swift/Xcode project present")}
	}

	// Go.
	if has("go.mod") {
		return Detection{"go", where("go.mod present")}
	}

	// ASP.NET / .NET.
	if glob("*.sln") || glob("*.csproj") {
		return Detection{"dotnet", where(".NET solution/project present")}
	}

	return Detection{"", ""}
}

// gradleUsesKotlin reports whether the Gradle build scripts reachable from dir
// reference the Kotlin plugin — the signal that separates a Kotlin Android
// project from a Java one. It reads the root build/settings scripts plus the
// conventional app/build.gradle (where the android plugin usually lives), since
// a root script alone often omits it. Best-effort: unreadable files are ignored.
func gradleUsesKotlin(dir string) bool {
	candidates := []string{
		"build.gradle",
		"settings.gradle",
		filepath.Join("app", "build.gradle"),
	}
	for _, rel := range candidates {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			continue
		}
		s := strings.ToLower(string(data))
		if strings.Contains(s, "kotlin") {
			return true
		}
	}
	return false
}

// rel renders root relative to the current working dir when possible, for a
// tidier reason string; falls back to the raw path.
func rel(root string) string {
	if wd, err := os.Getwd(); err == nil {
		if r, err := filepath.Rel(wd, root); err == nil {
			if r == "." {
				return "."
			}
			return "./" + filepath.ToSlash(r)
		}
	}
	return root
}

func depthWord() string {
	if maxDetectDepth == 1 {
		return "1"
	}
	return "2"
}
