package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// newBodyFlagCmd builds a minimal cobra.Command carrying the --body/--body-file
// flag pair, mirroring how pr.go and pr_edit.go register them.
func newBodyFlagCmd() (*cobra.Command, *string, *string) {
	var body, bodyFile string
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringVarP(&body, "body", "b", "", "")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "")
	return cmd, &body, &bodyFile
}

func TestResolveBodyOverride(t *testing.T) {
	bodyPath := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(bodyPath, []byte("body from file"), 0600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		args         []string
		stdin        string
		wantText     string
		wantProvided bool
		wantErr      bool
	}{
		{
			name:         "no flags",
			args:         nil,
			wantText:     "",
			wantProvided: false,
		},
		{
			name:         "body text",
			args:         []string{"--body", "inline description"},
			wantText:     "inline description",
			wantProvided: true,
		},
		{
			name:         "empty body still counts as provided",
			args:         []string{"--body", ""},
			wantText:     "",
			wantProvided: true,
		},
		{
			name:         "body from file",
			args:         []string{"--body-file", bodyPath},
			wantText:     "body from file",
			wantProvided: true,
		},
		{
			name:         "body from stdin",
			args:         []string{"--body-file", "-"},
			stdin:        "body from stdin",
			wantText:     "body from stdin",
			wantProvided: true,
		},
		{
			name:    "mutually exclusive",
			args:    []string{"--body", "a", "--body-file", bodyPath},
			wantErr: true,
		},
		{
			name:    "missing file",
			args:    []string{"--body-file", filepath.Join(t.TempDir(), "missing.md")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, body, bodyFile := newBodyFlagCmd()
			cmd.SetIn(strings.NewReader(tt.stdin))
			if err := cmd.Flags().Parse(tt.args); err != nil {
				t.Fatalf("parse flags: %v", err)
			}

			text, provided, err := resolveBodyOverride(cmd, *body, *bodyFile)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if text != tt.wantText || provided != tt.wantProvided {
				t.Fatalf("got (text=%q, provided=%v), want (text=%q, provided=%v)",
					text, provided, tt.wantText, tt.wantProvided)
			}
		})
	}
}
