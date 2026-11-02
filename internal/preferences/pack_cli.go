package preferences

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/BSH2409/hunkyard/internal/clicmd"
)

// runExportCLI handles `export --scope repository [--cwd DIR] --to FILE`:
// this repository's own rules as one markdown document to commit.
func runExportCLI(args []string, stdout io.Writer) int {
	emit := func(value any) { _ = json.NewEncoder(stdout).Encode(value) }
	parser := flag.NewFlagSet("export", flag.ContinueOnError)
	parser.SetOutput(io.Discard)
	root := parser.String("root", "", "Canonical preference root")
	parser.Bool("json", false, "JSON output")
	scope := parser.String("scope", "", "repository")
	cwd := parser.String("cwd", ".", "Repository directory")
	to := parser.String("to", "", "Markdown file to write")
	if parser.Parse(clicmd.ReorderFlagsFirst(args[1:], map[string]bool{"json": true})) != nil || parser.NArg() != 0 || *scope != "repository" || *to == "" {
		emit(map[string]any{"status": "usage", "error_class": "LearnPreferences::CLIUsageError"})
		return 2
	}
	s, err := Open(Config{Root: *root})
	if err == nil {
		var repoKey string
		if repoKey, err = RepositoryKey(*cwd); err == nil && repoKey == "" {
			err = fmt.Errorf("not inside a git repository")
		}
		if err == nil {
			err = os.WriteFile(*to, []byte(s.repositoryMarkdown(repoKey)), 0o644)
		}
	}
	if err != nil {
		emit(map[string]any{"status": "rejected", "error_class": "LearnPreferences::ValidationError", "error": err.Error()})
		return 3
	}
	emit(map[string]any{"status": "success", "to": *to})
	return 0
}

// runPackCLI handles `pack import|list|remove|export`.
//
//nolint:gocyclo // One case per pack subcommand keeps each beside its flags.
func runPackCLI(args []string, stdout io.Writer) int {
	emit := func(value any) { _ = json.NewEncoder(stdout).Encode(value) }
	usage := func() int {
		emit(map[string]any{"status": "usage", "error_class": "LearnPreferences::CLIUsageError"})
		return 2
	}
	if len(args) < 2 {
		return usage()
	}
	command := "pack " + args[1]
	parser := flag.NewFlagSet(command, flag.ContinueOnError)
	parser.SetOutput(io.Discard)
	root := parser.String("root", "", "Canonical preference root")
	parser.Bool("json", false, "JSON output")
	scope := parser.String("scope", "global", "global or repository")
	cwd := parser.String("cwd", "", "Repository directory for repository scope")
	repo := parser.Bool("repo", false, "Import the repository's own pack (.hunkyard/preferences-pack)")
	dryRun := parser.Bool("dry-run", false, "Report what would change without writing")
	name := parser.String("name", "", "Pack name (export)")
	version := parser.String("version", "", "Pack version (export)")
	author := parser.String("author", "", "Pack author (export)")
	out := parser.String("out", "", "Output folder (export)")
	if parser.Parse(clicmd.ReorderFlagsFirst(args[2:], map[string]bool{"json": true, "repo": true, "dry-run": true})) != nil {
		return usage()
	}
	if *scope != "global" && *scope != "repository" {
		return usage()
	}
	s, err := Open(Config{Root: *root})
	if err != nil {
		emit(map[string]any{"status": "rejected", "error_class": "LearnPreferences::ValidationError"})
		return 3
	}
	repoKeyFor := func() (string, error) {
		dir := *cwd
		if dir == "" {
			dir = "."
		}
		return RepositoryKey(dir)
	}
	var result any
	switch {
	case command == "pack import" && (parser.NArg() == 1 || *repo && parser.NArg() == 0):
		target := packTarget{scope: *scope}
		dir := parser.Arg(0)
		if *repo || *scope == "repository" {
			target.repoKey, err = repoKeyFor()
		}
		if err == nil && *repo {
			target.repository, target.scope = true, "repository"
			dir, err = repositoryPackDir(*cwd)
		}
		if err == nil {
			result, err = s.importPack(dir, target, *dryRun)
		}
	case command == "pack list" && parser.NArg() == 0:
		result = map[string]any{"status": "success", "packs": s.listPacks()}
	case command == "pack remove" && parser.NArg() == 1:
		result, err = s.removePack(parser.Arg(0))
	case command == "pack export" && parser.NArg() == 0 && *name != "" && *version != "" && *out != "":
		repoKey := ""
		if *scope == "repository" {
			repoKey, err = repoKeyFor()
		}
		if err == nil {
			result, err = s.exportPack(packExport{name: *name, version: *version, author: *author, scope: *scope, repoKey: repoKey, out: *out})
		}
	default:
		return usage()
	}
	if err != nil {
		emit(map[string]any{"status": "rejected", "error_class": "LearnPreferences::ValidationError", "error": err.Error()})
		return 3
	}
	emit(result)
	return 0
}
