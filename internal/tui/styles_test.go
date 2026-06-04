package tui

import (
	"fmt"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestNewStyles_FieldsRender exercises every Style in the palette by rendering
// the same sample string through it and asserting the output is non-empty. This
// is the minimal "construction succeeded" smoke test promised by the plan: each
// field is wired in and produces output.
func TestNewStyles_FieldsRender(t *testing.T) {
	t.Parallel()

	s := NewStyles()

	cases := []struct {
		name  string
		style lipgloss.Style
	}{
		{"Base", s.Base},
		{"Title", s.Title},
		{"TabActive", s.TabActive},
		{"TabInactive", s.TabInactive},
		{"Cursor", s.Cursor},
		{"Header", s.Header},
		{"Subtle", s.Subtle},
		{"Good", s.Good},
		{"Warn", s.Warn},
		{"Crit", s.Crit},
		{"Footer", s.Footer},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.style.Render("sample")
			if got == "" {
				t.Fatalf("Styles.%s.Render returned empty string", tc.name)
			}
		})
	}
}

// TestNewStyles_ColoredFieldsHaveForeground asserts the palette actually
// configures a foreground colour on every style that the docstrings claim
// should be coloured. Base is intentionally excluded because the docstring
// explicitly calls it "the catch-all default style"; everything else carries
// semantic colour from the TokyoNight palette.
func TestNewStyles_ColoredFieldsHaveForeground(t *testing.T) {
	t.Parallel()

	s := NewStyles()

	cases := []struct {
		name  string
		style lipgloss.Style
	}{
		{"Title", s.Title},
		{"TabActive", s.TabActive},
		{"TabInactive", s.TabInactive},
		{"Cursor", s.Cursor},
		{"Header", s.Header},
		{"Subtle", s.Subtle},
		{"Good", s.Good},
		{"Warn", s.Warn},
		{"Crit", s.Crit},
		{"Footer", s.Footer},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fg := tc.style.GetForeground()
			if fg == nil {
				t.Fatalf("Styles.%s.GetForeground returned nil interface", tc.name)
			}
			if _, ok := fg.(lipgloss.NoColor); ok {
				t.Fatalf("Styles.%s.GetForeground is NoColor; expected a TokyoNight palette colour", tc.name)
			}
		})
	}
}

// TestNewStyles_StatusColorsAreDistinct guards against an accidental copy-paste
// that would make the Good / Warn / Crit indicators visually identical. The
// peers and logs tabs rely on these three to convey status at a glance.
func TestNewStyles_StatusColorsAreDistinct(t *testing.T) {
	t.Parallel()

	s := NewStyles()

	good := fgKey(t, s.Good)
	warn := fgKey(t, s.Warn)
	crit := fgKey(t, s.Crit)

	if good == warn {
		t.Fatalf("Good and Warn share the same foreground colour %q", good)
	}
	if warn == crit {
		t.Fatalf("Warn and Crit share the same foreground colour %q", warn)
	}
	if good == crit {
		t.Fatalf("Good and Crit share the same foreground colour %q", good)
	}
}

// TestNewStyles_TabActiveIsDistinctFromInactive guards against the active tab
// being visually indistinguishable from inactive tabs in the tab bar.
func TestNewStyles_TabActiveIsDistinctFromInactive(t *testing.T) {
	t.Parallel()

	s := NewStyles()

	active := fgKey(t, s.TabActive)
	inactive := fgKey(t, s.TabInactive)

	if active == inactive {
		t.Fatalf("TabActive and TabInactive share the same foreground colour %q", active)
	}
}

// TestNewStyles_Independence verifies that NewStyles returns a fresh palette
// each call. This matters because the App threads the palette into every tab
// Model; if NewStyles returned a shared mutable backing it could leak edits
// between tests.
func TestNewStyles_Independence(t *testing.T) {
	t.Parallel()

	a := NewStyles()
	b := NewStyles()

	// Mutate via the value-receiver chainable API and confirm the original is
	// unchanged. lipgloss.Style is a value type so this should always hold,
	// but verifying it locks in the invariant for future refactors.
	mutated := a.Title.Foreground(lipgloss.Color("#000000"))
	if fgKey(t, a.Title) != fgKey(t, b.Title) {
		t.Fatalf("NewStyles returned palettes whose Title foregrounds diverge: a=%q b=%q",
			fgKey(t, a.Title), fgKey(t, b.Title))
	}
	if fmt.Sprintf("%v", mutated.GetForeground()) == fgKey(t, a.Title) {
		t.Fatalf("expected mutated.Title to differ from a.Title after Foreground change")
	}
}

// fgKey returns a stable string identity for a Style's foreground colour so
// tests can compare colours by equality without depending on the deprecated
// TerminalColor.RGBA() pathway (which resolves to 0,0,0 for hex Colors that
// haven't been routed through a renderer).
func fgKey(t *testing.T, s lipgloss.Style) string {
	t.Helper()
	fg := s.GetForeground()
	if fg == nil {
		t.Fatalf("Style.GetForeground returned nil interface")
	}
	return fmt.Sprintf("%T:%v", fg, fg)
}
