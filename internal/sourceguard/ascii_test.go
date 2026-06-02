// Package sourceguard holds a repo-wide source hygiene guard. It has no
// production code — only a test that every .go file in the module stays ASCII
// except for an explicitly sanctioned set of glyphs.
//
// Why: skillrig's human output deliberately uses a few non-ASCII glyphs (the
// "→" next-step hint, "✓/✗" verify status, the "—/·/…" separators). That set is
// intentional and kept (see issue #21 discussion). What we want to prevent is
// the ACCIDENTAL introduction of *other* non-ASCII runes — smart quotes from a
// copy-paste, a non-breaking space, a homoglyph — which render unpredictably in
// terminals/CI and are nearly invisible in review. golangci-lint cannot catch
// this: asciicheck only inspects identifiers, and bidichk only catches dangerous
// invisible/bidi runes, not ordinary glyphs in string literals.
//
// This test is the single source of truth for the allowlist. It runs as part of
// `go test ./...` (so `make check` and CI enforce it for free) and is also what
// the PostToolUse agent-loop hook invokes. To introduce a new glyph, add it to
// sanctioned below with a comment — a deliberate, reviewable edit.
package sourceguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sanctioned is the set of non-ASCII runes allowed anywhere in .go source. Each
// entry is intentional; adding one is a deliberate decision recorded here.
var sanctioned = map[rune]string{
	'→': "U+2192 next-step footer hints (init/add/search/index/verify)",
	'✓': "U+2713 verify OK status",
	'✗': "U+2717 verify failure status",
	'—': "U+2014 em dash — prose/help/output separator",
	'·': "U+00B7 search footer separator (N skill(s) · run:)",
	'…': "U+2026 truncation/elision marker",
	'•': "U+2022 bullet in `add` help text",
	'§': "U+00A7 section sign in Constitution references (comments)",
	'≤': "U+2264 bounded-output notes in comments (≤2 lines)",
	'–': "U+2013 en dash in numeric ranges in comments (rows 1–7)",
	'⇒': "U+21D2 logical implication in a comment",
}

// TestSourceIsASCIIExceptSanctioned walks the module and fails on any non-ASCII
// rune that is not in sanctioned, reporting file:line:col and the offending rune
// so the fix is obvious: either revert to ASCII, or (if genuinely intended) add
// the rune to sanctioned.
func TestSourceIsASCIIExceptSanctioned(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)

	var violations []string

	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		violations = append(violations, scanFile(t, path)...)

		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking module from %q: %v", root, walkErr)
	}

	if len(violations) > 0 {
		t.Fatalf("found %d unsanctioned non-ASCII rune(s) in .go source:\n%s\n\n"+
			"fix: revert to ASCII, or if the glyph is intentional add it to "+
			"sanctioned in internal/sourceguard/ascii_test.go with a justification.",
			len(violations), strings.Join(violations, "\n"))
	}
}

// scanFile returns one violation string per unsanctioned non-ASCII rune in the
// file at path, tracking the path relative to the module root for readability.
func scanFile(t *testing.T, path string) []string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %q: %v", path, err)
	}

	var out []string

	rel := relOrPath(path)

	for i, text := range strings.Split(string(data), "\n") {
		col := 0
		for _, r := range text {
			col++

			if r < 128 {
				continue
			}

			if _, ok := sanctioned[r]; ok {
				continue
			}

			out = append(out, formatViolation(rel, i+1, col, r))
		}
	}

	return out
}
