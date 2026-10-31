package preferences

import (
	"path"
	"regexp"
	"strings"
)

// gitAction is a git commit or push found in a shell command.
type gitAction struct {
	Verb    string
	Dir     string
	Message string
	All     bool
	Args    []string
}

// shellSegments splits a shell command into simple commands, honouring
// quotes and backslash escapes, at &&, ||, ;, | and newlines.
func shellSegments(command string) [][]string {
	var segments [][]string
	var words []string
	var word strings.Builder
	inWord := false
	flushWord := func() {
		if inWord {
			words = append(words, word.String())
			word.Reset()
			inWord = false
		}
	}
	flushSegment := func() {
		flushWord()
		if len(words) > 0 {
			segments = append(segments, words)
			words = nil
		}
	}
	runes := []rune(command)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch {
		case c == '\'':
			inWord = true
			for i++; i < len(runes) && runes[i] != '\''; i++ {
				word.WriteRune(runes[i])
			}
		case c == '"':
			inWord = true
			for i++; i < len(runes) && runes[i] != '"'; i++ {
				if runes[i] == '\\' && i+1 < len(runes) && strings.ContainsRune(`"\$`+"`", runes[i+1]) {
					i++
				}
				word.WriteRune(runes[i])
			}
		case c == '\\' && i+1 < len(runes):
			inWord = true
			i++
			word.WriteRune(runes[i])
		case c == ';' || c == '\n' || c == '|' || c == '&':
			flushSegment()
		case c == ' ' || c == '\t':
			flushWord()
		default:
			inWord = true
			word.WriteRune(c)
		}
	}
	flushSegment()
	return segments
}

var envAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

var shellInterpreters = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true}

// simpleCommands calls fn with each simple command in a shell command, after
// stripping env and VAR=value prefixes and unwrapping `sh -c SCRIPT`. dir is
// the directory a preceding `cd` moved to.
func simpleCommands(command string, fn func(words []string, dir string) bool) bool {
	dir := ""
	for _, words := range shellSegments(command) {
		for len(words) > 0 && (envAssignment.MatchString(words[0]) || path.Base(words[0]) == "env") {
			words = words[1:]
		}
		if len(words) == 0 {
			continue
		}
		if words[0] == "cd" && len(words) > 1 {
			dir = words[1]
			continue
		}
		if shellInterpreters[path.Base(words[0])] {
			for i := 1; i+1 < len(words); i++ {
				if strings.HasPrefix(words[i], "-") && strings.HasSuffix(words[i], "c") {
					if simpleCommands(words[i+1], fn) {
						return true
					}
					break
				}
			}
			continue
		}
		if fn(words, dir) {
			return true
		}
	}
	return false
}

// parseGitInvocation finds the first git commit or push in a shell command,
// following a preceding `cd DIR` and `git -C DIR`.
func parseGitInvocation(command string) (gitAction, bool) {
	var found gitAction
	ok := simpleCommands(command, func(words []string, dir string) bool {
		if path.Base(words[0]) != "git" {
			return false
		}
		action := gitAction{Dir: dir}
		i := 1
		for i < len(words) && strings.HasPrefix(words[i], "-") {
			switch words[i] {
			case "-C":
				if i+1 < len(words) {
					action.Dir = words[i+1]
					i++
				}
			case "-c":
				i++
			}
			i++
		}
		if i >= len(words) || words[i] != "commit" && words[i] != "push" {
			return false
		}
		action.Verb = words[i]
		action.Args = words[i+1:]
		if action.Verb == "commit" {
			parseCommitArgs(&action)
		}
		found = action
		return true
	})
	return found, ok
}

// runsPreferenceOverride reports whether a shell command would grant an
// override or switch off enforcement: `preferences allow`, `guard remove`
// or `retire`. Only the user may do those.
func runsPreferenceOverride(command string) bool {
	return simpleCommands(command, func(words []string, _ string) bool {
		for i := 0; i+1 < len(words); i++ {
			if words[i] != "preferences" {
				continue
			}
			switch words[i+1] {
			case "allow", "retire":
				return true
			case "guard":
				return i+2 < len(words) && words[i+2] == "remove"
			}
		}
		return false
	})
}

func parseCommitArgs(action *gitAction) {
	var messages []string
	args := action.Args
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-m" || arg == "--message":
			if i+1 < len(args) {
				messages = append(messages, args[i+1])
				i++
			}
		case strings.HasPrefix(arg, "--message="):
			messages = append(messages, strings.TrimPrefix(arg, "--message="))
		case arg == "--all":
			action.All = true
		case strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--"):
			flags := arg[1:]
			if strings.Contains(flags, "a") {
				action.All = true
			}
			if strings.HasSuffix(flags, "m") && i+1 < len(args) {
				messages = append(messages, args[i+1])
				i++
			} else if j := strings.Index(flags, "m"); j >= 0 && j < len(flags)-1 {
				messages = append(messages, flags[j+1:])
			}
		}
	}
	for i, message := range messages {
		messages[i] = substitutedMessage(message)
	}
	action.Message = strings.Join(messages, "\n\n")
}

// substitutedMessage resolves `$(cat <<'TAG' … TAG)`, the form agents use for
// multi-line commit messages. Any other command substitution yields "", so
// format checks are skipped rather than run on shell syntax.
func substitutedMessage(message string) string {
	trimmed := strings.TrimSpace(message)
	if !strings.HasPrefix(trimmed, "$(") && !strings.Contains(trimmed, "`") {
		return message
	}
	marker := strings.Index(trimmed, "<<")
	newline := strings.IndexByte(trimmed, '\n')
	if !strings.HasPrefix(trimmed, "$(cat") || marker < 0 || newline < marker {
		return ""
	}
	tag := strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed[marker+2:newline], "-")), `'"`)
	body := trimmed[newline+1:]
	end := strings.Index(body, "\n"+tag)
	if strings.HasPrefix(body, tag) {
		end = 0
	}
	if tag == "" || end < 0 {
		return ""
	}
	return strings.TrimRight(body[:end], "\n")
}

// matchGuardPattern matches a repository-relative path against a
// gitignore-style glob; a pattern without a slash matches the file name.
func matchGuardPattern(pattern, name string) bool {
	if !strings.Contains(pattern, "/") {
		name = path.Base(name)
	}
	var expr strings.Builder
	expr.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			expr.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(pattern[i:], "**"):
			expr.WriteString(".*")
			i++
		case pattern[i] == '*':
			expr.WriteString("[^/]*")
		case pattern[i] == '?':
			expr.WriteString("[^/]")
		default:
			expr.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		}
	}
	expr.WriteString("$")
	matched, err := regexp.MatchString(expr.String(), name)
	return err == nil && matched
}
