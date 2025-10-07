package testutil

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	mib_parser "github.com/Olian04/go-mib-parser"
	coreparser "github.com/Olian04/go-mib-parser/parser"
)

type Expected struct {
	ObjectIdentifierNames map[string]struct{}
	ObjectTypeNames       map[string]struct{}
	ModuleIdentityName    string
	ObjectIdentityNames   map[string]struct{}
	NotificationTypeNames map[string]struct{}
}

// VerifyMIB parses a MIB with the public API and internal IR and verifies
// that all OID-bearing constructs present in the source appear in parsed output.
func VerifyMIB(t *testing.T, mibSource []byte, filename string) {
	t.Helper()

	// Strip ASN.1 line comments ("-- ...") for easier regex matching.
	cleaned := stripLineComments(string(mibSource))
	cleaned = removeQuotedStrings(cleaned)
	cleaned = removeImportsBlocks(cleaned)
	exp := extractExpected(cleaned)

	// Public API must succeed
	mod, err := mib_parser.ParseMIB(mibSource)
	if err != nil {
		t.Fatalf("ParseMIB failed for %s: %v", filename, err)
	}
	if mod == nil || mod.Name == "" {
		t.Fatalf("Parsed module was empty for %s", filename)
	}

	// Internal IR for node-level validation
	ir, err := coreparser.Parse(mibSource)
	if err != nil {
		t.Fatalf("internal Parse failed for %s: %v", filename, err)
	}

	// OBJECT IDENTIFIER names present in IR
	for name := range exp.ObjectIdentifierNames {
		if _, ok := ir.NodesByName[name]; !ok {
			t.Errorf("expected OBJECT IDENTIFIER node %q to be present in parsed IR (%s)", name, filename)
		}
	}

	// OBJECT-TYPE names present in public API
	for name := range exp.ObjectTypeNames {
		if _, ok := mod.ObjectsByName[name]; !ok {
			t.Errorf("expected OBJECT-TYPE %q to be present in parsed module (%s)", name, filename)
		}
	}

	// MODULE-IDENTITY presence (name must match if declared)
	if exp.ModuleIdentityName != "" {
		if mod.ModuleIdentity == nil || !equalFold(mod.ModuleIdentity.Name, exp.ModuleIdentityName) {
			t.Errorf("expected MODULE-IDENTITY %q to be present (got %v) in %s", exp.ModuleIdentityName, nameOrEmpty(mod.ModuleIdentity), filename)
		}
	}

	// OBJECT-IDENTITY names present in public API
	for name := range exp.ObjectIdentityNames {
		if _, ok := mod.ObjectIdentities[name]; !ok {
			t.Errorf("expected OBJECT-IDENTITY %q to be present in parsed module (%s)", name, filename)
		}
	}

	// NOTIFICATION-TYPE names present in public API
	for name := range exp.NotificationTypeNames {
		if _, ok := mod.NotificationTypes[name]; !ok {
			t.Errorf("expected NOTIFICATION-TYPE %q to be present in parsed module (%s)", name, filename)
		}
	}
}

func nameOrEmpty(mi *mib_parser.ModuleIdentity) string {
	if mi == nil {
		return "<nil>"
	}
	return mi.Name
}

func extractExpected(src string) Expected {
	// Regexes are case-sensitive per SMIv2 keywords; keep uppercase matches
	// Capture leading identifier before keyword
	reObjId := regexp.MustCompile(`(?m)^\s*([A-Za-z][A-Za-z0-9-]*)\s+OBJECT\s+IDENTIFIER\s+::=`)
	reObjType := regexp.MustCompile(`(?m)^\s*([A-Za-z][A-Za-z0-9-]*)\s+OBJECT-TYPE\b`)
	reModId := regexp.MustCompile(`(?m)^\s*([A-Za-z][A-Za-z0-9-]*)\s+MODULE-IDENTITY\b`)
	reObjIdentity := regexp.MustCompile(`(?m)^\s*([A-Za-z][A-Za-z0-9-]*)\s+OBJECT-IDENTITY\b`)
	reNotif := regexp.MustCompile(`(?m)^\s*([A-Za-z][A-Za-z0-9-]*)\s+NOTIFICATION-TYPE\b`)

	out := Expected{
		ObjectIdentifierNames: map[string]struct{}{},
		ObjectTypeNames:       map[string]struct{}{},
		ObjectIdentityNames:   map[string]struct{}{},
		NotificationTypeNames: map[string]struct{}{},
	}
	for _, m := range reObjId.FindAllStringSubmatch(src, -1) {
		out.ObjectIdentifierNames[m[1]] = struct{}{}
	}
	for _, m := range reObjType.FindAllStringSubmatch(src, -1) {
		name := m[1]
		if isReserved(name) {
			continue
		}
		out.ObjectTypeNames[name] = struct{}{}
	}
	if m := reModId.FindStringSubmatch(src); len(m) == 2 {
		out.ModuleIdentityName = m[1]
	}
	for _, m := range reObjIdentity.FindAllStringSubmatch(src, -1) {
		name := m[1]
		if isReserved(name) {
			continue
		}
		out.ObjectIdentityNames[name] = struct{}{}
	}
	for _, m := range reNotif.FindAllStringSubmatch(src, -1) {
		name := m[1]
		if isReserved(name) {
			continue
		}
		out.NotificationTypeNames[name] = struct{}{}
	}
	return out
}

func stripLineComments(src string) string {
	// Remove "-- ..." to EOL, but keep text before comments
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

// removeImportsBlocks removes text between IMPORTS and the following ';' to avoid
// regex matches on keywords listed in IMPORTS (e.g., MODULE-IDENTITY).
func removeImportsBlocks(src string) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	skipping := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !skipping && strings.HasPrefix(trimmed, "IMPORTS") {
			// start skipping until we see a ';'
			if strings.Contains(line, ";") {
				// single-line IMPORTS; drop it
				continue
			}
			skipping = true
			continue
		}
		if skipping {
			if strings.Contains(line, ";") {
				skipping = false
			}
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// removeQuotedStrings strips content inside double quotes to avoid
// treating examples inside DESCRIPTION as real definitions.
func removeQuotedStrings(src string) string {
	runes := []rune(src)
	out := make([]rune, 0, len(runes))
	in := false
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '"' {
			in = !in
			// keep the quote as whitespace boundary
			out = append(out, ' ')
			continue
		}
		if in {
			// preserve newlines for line structure, blank out others
			if r == '\n' || r == '\r' {
				out = append(out, r)
			} else {
				out = append(out, ' ')
			}
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

func equalFold(a, b string) bool {
	return strings.EqualFold(a, b)
}

func isReserved(s string) bool {
	switch s {
	case "BEGIN", "END", "DEFINITIONS", "IMPORTS":
		return true
	default:
		return false
	}
}

// Helper to get test path for a MIB under ../mibs
func MIBPath(name string) string {
	return filepath.Join("..", "mibs", name)
}

// ReadMIB uses the embedded FS to read a MIB by name from mibs/.
func ReadMIB(t *testing.T, name string) []byte {
	t.Helper()
	// Try a set of likely relative paths depending on the test package dir
	candidates := []string{
		filepath.Join("..", "..", "mibs", name), // when running from tests/mibs_test
		filepath.Join("..", "mibs", name),       // when running from tests/
		filepath.Join("mibs", name),             // when running from repo root
	}
	var lastErr error
	for _, p := range candidates {
		if b, err := os.ReadFile(p); err == nil {
			return b
		} else {
			lastErr = err
		}
	}
	t.Fatalf("failed to read MIB %s; searched: %s; last error: %v", name, strings.Join(candidates, ", "), lastErr)
	return nil
}
