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
	}{
		{name: "valid", in: "my-org/my-skills", wantOwner: "my-org", wantRepo: "my-skills"},
		{name: "valid with dots and underscores", in: "my.org_1/skills.v2_x", wantOwner: "my.org_1", wantRepo: "skills.v2_x"},
		{name: "surrounding whitespace trimmed", in: "  my-org/my-skills\n", wantOwner: "my-org", wantRepo: "my-skills"},
		{name: "empty", in: "", wantErr: true},
		{name: "blank whitespace", in: "   ", wantErr: true},
		{name: "no slash", in: "my-org-my-skills", wantErr: true},
		{name: "missing repo", in: "my-org/", wantErr: true},
		{name: "missing owner", in: "/my-skills", wantErr: true},
		{name: "too many segments", in: "my-org/team/skills", wantErr: true},
		{name: "illegal char", in: "my org/my skills", wantErr: true},
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

			if got.Owner != tc.wantOwner || got.Repo != tc.wantRepo {
				t.Errorf("ParseOrigin(%q) = %+v, want {Owner:%q Repo:%q}", tc.in, got, tc.wantOwner, tc.wantRepo)
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

	o := Origin{Owner: "my-org", Repo: "my-skills"}
	if got := o.String(); got != "my-org/my-skills" {
		t.Errorf("String() = %q, want %q", got, "my-org/my-skills")
	}
}
