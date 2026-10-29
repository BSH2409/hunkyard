package preferences

import (
	"crypto/sha256"
	"encoding/hex"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func digest(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}

var labelCharacters = regexp.MustCompile(`[^a-z0-9]+`)
var remoteCredentials = regexp.MustCompile(`(?i)^([a-z][a-z0-9+.-]*://)[^/@\s]+@`)
var remoteSSH = regexp.MustCompile(`^[^/@\s]+@([^:]+):`)

func repositoryLabel(path string) string {
	name := filepath.Base(strings.TrimRight(path, "/"))
	name = strings.TrimSuffix(strings.TrimSuffix(name, ".git"), ".GIT")
	name = strings.Trim(labelCharacters.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if name == "" {
		return "repository"
	}
	return truncate(name, 64)
}

func repositoryKey(cwd string) (string, error) {
	root, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	run := func(args ...string) string {
		data, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(data))
	}
	if common := run("rev-parse", "--git-common-dir"); common != "" {
		if !filepath.IsAbs(common) {
			common = filepath.Join(root, common)
		}
		common = filepath.Clean(common)
		labelPath := common
		if filepath.Base(common) == ".git" {
			labelPath = filepath.Dir(common)
		}
		return repositoryLabel(labelPath) + "-" + digest(common)[:12], nil
	}
	if remote := run("remote", "get-url", "origin"); remote != "" {
		remote = remoteCredentials.ReplaceAllString(remote, "${1}")
		remote = remoteSSH.ReplaceAllString(remote, "${1}/")
		remote = strings.TrimRight(remote, "/")
		if strings.HasSuffix(strings.ToLower(remote), ".git") {
			remote = remote[:len(remote)-4]
		}
		if remote != "" {
			return repositoryLabel(remote) + "-" + digest(remote)[:12], nil
		}
	}
	if top := run("rev-parse", "--show-toplevel"); top != "" {
		return repositoryLabel(top) + "-" + digest(top)[:12], nil
	}
	return "", nil
}
