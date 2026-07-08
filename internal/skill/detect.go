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

// Detect inspects root for signature files and returns the stack (pack name).
// This is pure, deterministic file inspection — no reasoning. Order matters:
// more specific signals (react-native inside package.json) are checked before
// generic ones (react). Returns an empty Stack if nothing matches.
func Detect(root string) Detection {
	has := func(name string) bool {
		_, err := os.Stat(filepath.Join(root, name))
		return err == nil
	}
	glob := func(pattern string) bool {
		m, _ := filepath.Glob(filepath.Join(root, pattern))
		return len(m) > 0
	}

	// Flutter: pubspec.yaml is unambiguous.
	if has("pubspec.yaml") {
		return Detection{"flutter", "pubspec.yaml present"}
	}

	// JS ecosystem: distinguish React Native from React by package.json contents.
	if has("package.json") {
		pkg, _ := os.ReadFile(filepath.Join(root, "package.json"))
		s := string(pkg)
		switch {
		case strings.Contains(s, "\"react-native\""):
			return Detection{"rn", "react-native in package.json"}
		case strings.Contains(s, "\"react\""):
			return Detection{"react", "react in package.json"}
		}
	}

	// Android native (Kotlin/Java).
	if has("build.gradle") || has("build.gradle.kts") || has("settings.gradle") || has("settings.gradle.kts") {
		return Detection{"kotlin", "gradle build files present"}
	}

	// iOS native (Swift).
	if has("Package.swift") || glob("*.xcodeproj") || glob("*.xcworkspace") {
		return Detection{"swift", "Swift/Xcode project present"}
	}

	// Go.
	if has("go.mod") {
		return Detection{"go", "go.mod present"}
	}

	// ASP.NET / .NET.
	if glob("*.sln") || glob("*.csproj") {
		return Detection{"dotnet", ".NET solution/project present"}
	}

	return Detection{"", "no known stack signature found"}
}
