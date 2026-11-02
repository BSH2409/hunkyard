package preferences

import "testing"

func TestRequestsCommit(t *testing.T) {
	tests := []struct {
		prompt string
		want   bool
	}{
		{"commit this", true},
		{"push the branch", true},
		{"make a commit", true},
		{"create a commit for the fix", true},
		{"don't forget to commit", true},
		{"No, just commit it", true},
		{"open a PR and merge then lets create a new branch from main", true},
		{"create a pull request", true},
		{"merge it into main", true},
		{"don't commit yet", false},
		{"do not push", false},
		{"never commit plan files", false},
		{"the last commit broke tests", false},
		{"what does this commit change?", false},
		{"don't open a PR", false},
		{"explain the merge conflict", false},
	}
	for _, tt := range tests {
		if got := requestsCommit(tt.prompt); got != tt.want {
			t.Errorf("requestsCommit(%q) = %v, want %v", tt.prompt, got, tt.want)
		}
	}
}
