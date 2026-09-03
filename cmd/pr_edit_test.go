package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewPREditCmd(t *testing.T) {
	bodyPath := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(bodyPath, []byte("body from file"), 0600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name            string
		args            []string
		stdin           string
		wantSelector    string
		wantTitle       string
		wantTitleEdited bool
		wantBody        string
		wantBodyEdited  bool
		wantErr         bool
	}{
		{
			name:            "title and body",
			args:            []string{"564", "--title", "New title", "--body", "New body"},
			wantSelector:    "564",
			wantTitle:       "New title",
			wantTitleEdited: true,
			wantBody:        "New body",
			wantBodyEdited:  true,
		},
		{
			name:           "body from stdin",
			args:           []string{"564", "--body-file", "-"},
			stdin:          "body from stdin",
			wantSelector:   "564",
			wantBody:       "body from stdin",
			wantBodyEdited: true,
		},
		{
			name:           "body from file",
			args:           []string{"feature/example", "--body-file", bodyPath},
			wantSelector:   "feature/example",
			wantBody:       "body from file",
			wantBodyEdited: true,
		},
		{
			name:           "empty body",
			args:           []string{"564", "--body", ""},
			wantSelector:   "564",
			wantBodyEdited: true,
		},
		{
			name:    "body flags conflict",
			args:    []string{"564", "--body", "text", "--body-file", bodyPath},
			wantErr: true,
		},
		{
			name:    "no edits",
			args:    []string{"564"},
			wantErr: true,
		},
		{
			name:    "too many selectors",
			args:    []string{"564", "565", "--title", "New title"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *prEditOptions
			cmd := newPREditCmd(func(opts *prEditOptions) error {
				got = opts
				return nil
			})
			cmd.SetArgs(tt.args)
			cmd.SetIn(strings.NewReader(tt.stdin))

			err := cmd.Execute()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("edit runner was not called")
			}
			if got.selector != tt.wantSelector || got.title != tt.wantTitle ||
				got.titleEdited != tt.wantTitleEdited || got.body != tt.wantBody ||
				got.bodyEdited != tt.wantBodyEdited {
				t.Fatalf("options = %+v", got)
			}
		})
	}
}
