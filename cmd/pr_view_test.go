package cmd

import (
	"testing"

	"github.com/chinhstringee/buck/internal/bitbucket"
)

func TestFormatCommitHash(t *testing.T) {
	t.Cleanup(func() { prViewFlagFull = false })

	tests := []struct {
		name   string
		commit *bitbucket.PRCommitRef
		full   bool
		want   string
	}{
		{"nil commit", nil, false, "(unknown)"},
		{"empty hash", &bitbucket.PRCommitRef{Hash: ""}, false, "(unknown)"},
		{"short by default", &bitbucket.PRCommitRef{Hash: "abcdef1234567890"}, false, "abcdef1"},
		{"full when requested", &bitbucket.PRCommitRef{Hash: "abcdef1234567890"}, true, "abcdef1234567890"},
		{"hash shorter than 7 stays as-is", &bitbucket.PRCommitRef{Hash: "abc"}, false, "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prViewFlagFull = tt.full
			got := formatCommitHash(tt.commit)
			if got != tt.want {
				t.Errorf("formatCommitHash() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatReviewers(t *testing.T) {
	tests := []struct {
		name         string
		participants []bitbucket.PRParticipant
		want         string
	}{
		{"no reviewers", nil, "(none)"},
		{
			"non-reviewer participants excluded",
			[]bitbucket.PRParticipant{{Role: "PARTICIPANT", User: bitbucket.PRAuthor{DisplayName: "Alice"}}},
			"(none)",
		},
		{
			"approved and pending reviewers",
			[]bitbucket.PRParticipant{
				{Role: "REVIEWER", Approved: true, User: bitbucket.PRAuthor{DisplayName: "Alice"}},
				{Role: "REVIEWER", Approved: false, User: bitbucket.PRAuthor{DisplayName: "Bob"}},
			},
			"Alice (approved), Bob (pending)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatReviewers(tt.participants)
			if got != tt.want {
				t.Errorf("formatReviewers() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunPRView_RequiresBranchOrID(t *testing.T) {
	prViewFlagID = 0
	t.Cleanup(func() { prViewFlagID = 0 })

	err := runPRView(prViewCmd, nil)
	if err == nil {
		t.Fatal("expected error when neither branch nor --id is given")
	}
}
