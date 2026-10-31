package preferences

import "testing"

func TestParseGitInvocation(t *testing.T) {
	tests := []struct {
		command string
		ok      bool
		want    gitAction
	}{
		{`git commit -m "x"`, true, gitAction{Verb: "commit", Message: "x"}},
		{`git -C /r commit -am "x"`, true, gitAction{Verb: "commit", Dir: "/r", Message: "x", All: true}},
		{`cd /r && git commit -m a -m b`, true, gitAction{Verb: "commit", Dir: "/r", Message: "a\n\nb"}},
		{`git commit --message="fix; it"`, true, gitAction{Verb: "commit", Message: "fix; it"}},
		{`GIT_AUTHOR_NAME=x git push origin main`, true, gitAction{Verb: "push"}},
		{`go test ./... && git push`, true, gitAction{Verb: "push"}},
		{`echo "git commit"`, false, gitAction{}},
		{`grep commit file`, false, gitAction{}},
		{`git status`, false, gitAction{}},
	}
	for _, tt := range tests {
		got, ok := parseGitInvocation(tt.command)
		if ok != tt.ok || ok && (got.Verb != tt.want.Verb || got.Dir != tt.want.Dir || got.Message != tt.want.Message || got.All != tt.want.All) {
			t.Errorf("parseGitInvocation(%q) = %+v, %v; want %+v, %v", tt.command, got, ok, tt.want, tt.ok)
		}
	}
}

func TestGlobMatchesRepositoryPaths(t *testing.T) {
	tests := []struct {
		pattern, path string
		want          bool
	}{
		{"docs/superpowers/**", "docs/superpowers/plans/a.md", true},
		{"*-plan.md", "notes/release-plan.md", true},
		{"spec/**", "spec/models/user_spec.rb", true},
		{"spec/**", "app/spec.rb", false},
		{"*-plan.md", "planner.md", false},
	}
	for _, tt := range tests {
		if got := matchGuardPattern(tt.pattern, tt.path); got != tt.want {
			t.Errorf("matchGuardPattern(%q, %q) = %v", tt.pattern, tt.path, got)
		}
	}
}
