package config

import (
	"strings"
	"testing"
)

func TestParseOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		in        string
		wantErr   bool
		wantOwner string
		wantRepo  string
		wantRef   string
	}{
		{name: "valid", in: "my-org/my-skills", wantOwner: "my-org", wantRepo: "my-skills"},
		{name: "valid with dots and underscores", in: "my.org_1/skills.v2_x", wantOwner: "my.org_1", wantRepo: "skills.v2_x"},
		{name: "surrounding whitespace trimmed", in: "  my-org/my-skills\n", wantOwner: "my-org", wantRepo: "my-skills"},
		{name: "branch ref", in: "my-org/my-skills@main", wantOwner: "my-org", wantRepo: "my-skills", wantRef: "main"},
		{name: "tag ref", in: "my-org/my-skills@v1.4.0", wantOwner: "my-org", wantRepo: "my-skills", wantRef: "v1.4.0"},
		{name: "commit ref", in: "my-org/my-skills@9f2c1a0", wantOwner: "my-org", wantRepo: "my-skills", wantRef: "9f2c1a0"},
		{name: "branch ref with slash", in: "my-org/my-skills@feature/auth", wantOwner: "my-org", wantRepo: "my-skills", wantRef: "feature/auth"},
		{name: "ref with surrounding whitespace trimmed", in: "  my-org/my-skills@staging\n", wantOwner: "my-org", wantRepo: "my-skills", wantRef: "staging"},
		{name: "empty", in: "", wantErr: true},
		{name: "blank whitespace", in: "   ", wantErr: true},
		{name: "no slash", in: "my-org-my-skills", wantErr: true},
		{name: "missing repo", in: "my-org/", wantErr: true},
		{name: "missing owner", in: "/my-skills", wantErr: true},
		{name: "too many segments", in: "my-org/team/skills", wantErr: true},
		{name: "illegal char", in: "my org/my skills", wantErr: true},
		{name: "trailing at with empty ref", in: "my-org/my-skills@", wantErr: true},
		{name: "ref with space", in: "my-org/my-skills@bad ref", wantErr: true},
		{name: "ref with illegal char", in: "my-org/my-skills@bad:ref", wantErr: true},
		{name: "at without owner repo", in: "@main", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseOrigin(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseOrigin(%q) = %v, want error", tc.in, got)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseOrigin(%q) unexpected error: %v", tc.in, err)
			}

			if got.Owner != tc.wantOwner || got.Repo != tc.wantRepo || got.Ref != tc.wantRef {
				t.Errorf("ParseOrigin(%q) = %+v, want {Owner:%q Repo:%q Ref:%q}", tc.in, got, tc.wantOwner, tc.wantRepo, tc.wantRef)
			}
		})
	}
}

func TestParseOriginErrorEchoesValue(t *testing.T) {
	t.Parallel()

	_, err := ParseOrigin("not-valid")
	if err == nil {
		t.Fatal("expected error for malformed origin")
	}
	// FR-012: error names the expected format and echoes the offending value.
	msg := err.Error()
	for _, want := range []string{"not-valid", "OWNER/REPO"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestOriginString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   Origin
		want string
	}{
		{name: "no ref", in: Origin{Owner: "my-org", Repo: "my-skills"}, want: "my-org/my-skills"},
		{name: "with ref", in: Origin{Owner: "my-org", Repo: "my-skills", Ref: "main"}, want: "my-org/my-skills@main"},
		{name: "with slash ref", in: Origin{Owner: "my-org", Repo: "my-skills", Ref: "feature/auth"}, want: "my-org/my-skills@feature/auth"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.in.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestParseOriginStringRoundTrip pins that an origin with a ref survives the
// String() → ParseOrigin round-trip unchanged — the property that lets the ref
// be stored inside the single `origin` config key with no resolver change
// (amendment 001-origin-ref-support, FR-020).
func TestParseOriginStringRoundTrip(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"my-org/my-skills", "my-org/my-skills@main", "my-org/my-skills@feature/auth"} {
		o, err := ParseOrigin(in)
		if err != nil {
			t.Fatalf("ParseOrigin(%q): %v", in, err)
		}

		if got := o.String(); got != in {
			t.Errorf("round-trip ParseOrigin(%q).String() = %q, want %q", in, got, in)
		}
	}
}

// TestOriginStringZeroIsEmpty pins Qodo #3: the zero Origin (the SourceNone
// sentinel returned by ResolveOrigin) must stringify to "" — never "/" — so it
// cannot be mistaken for a configured value.
func TestOriginStringZeroIsEmpty(t *testing.T) {
	t.Parallel()

	if got := (Origin{}).String(); got != "" {
		t.Errorf("zero Origin String() = %q, want empty string", got)
	}
}
