package comment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/review"
	"github.com/BSH2409/hunkyard/internal/session"
)

// isAbsoluteOrTraversal reports whether p looks like an absolute path or
// escapes via "..". Checks both POSIX (/foo) and host (C:\\foo) notations,
// so the validator rejects /etc/passwd-style inputs on Windows where
// filepath.IsAbs("/foo") returns false. Run on the raw input — after
// filepath.Clean a leading "/" can be rewritten to "\\" on Windows and
// the prefix check would silently miss the attack.
func isAbsoluteOrTraversal(p string) bool {
	if filepath.IsAbs(p) {
		return true
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return true
	}
	cleaned := filepath.Clean(p)
	return strings.HasPrefix(cleaned, "..")
}

// checkCommentCLIAllowed returns an error if the review at hunkyardPath is a
// live or preview review. Those reviews pin DOM elements, so the file/line
// form does not apply. --selector adds a pin instead. form names the
// comment the user tried to add (see commentFormFor).
func checkCommentCLIAllowed(hunkyardPath, form string) error {
	data, err := os.ReadFile(review.ReviewPathsFor(hunkyardPath).Review)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var cj session.HunkyardJSON
	if err := json.Unmarshal(data, &cj); err != nil {
		return nil //nolint:nilerr // malformed review file: do not block CLI
	}
	if cj.ReviewType == "live" || cj.ReviewType == "preview" {
		return fmt.Errorf("%s is not supported for %s reviews; use: hunkyard comment --selector <css> [--route <path>] <body>", form, cj.ReviewType)
	}
	return nil
}

// Comment forms named in the live/preview rejection, so the error matches
// what the user ran.
const (
	formReviewComment = "a review-level comment (hunkyard comment <body>)"
	formFileComment   = "a file comment (hunkyard comment <path> <body>)"
	formLineComment   = "a line comment (hunkyard comment <path>:<line> <body>)"
)

// commentFormFor names the comment form args would create. args has at
// least one element.
func commentFormFor(args []string) string {
	if len(args) == 1 {
		return formReviewComment
	}
	loc := args[0]
	if i := strings.LastIndex(loc, ":"); i > 0 && looksLikeLineSpec(loc[i+1:]) {
		return formLineComment
	}
	return formFileComment
}

// bulkEntryForm names the comment form a JSON entry without a selector would
// create, in the same words as commentFormFor.
func bulkEntryForm(e BulkCommentEntry) string {
	switch {
	case e.Line > 0 || e.LineSpec != "":
		return formLineComment
	case e.File != "" || e.Path != "":
		return formFileComment
	default:
		return formReviewComment
	}
}

// appendCommentScoped adds a comment to the session.HunkyardJSON struct in memory with
// HeadSHA / DiffScope stamping. Does not write to disk.
// scope.DiffScope == "" produces today's working-tree behavior.
func appendCommentScoped(cj *session.HunkyardJSON, filePath string, startLine, endLine int, body, author string, scope session.InheritedScope) {
	appendQuotedCommentScoped(cj, filePath, startLine, endLine, body, "", nil, author, scope)
}

// appendQuotedCommentScoped is appendCommentScoped for a comment on part of
// the line range: quote is the selected text and quoteOffset, when set, its
// position as the web UI records it (see session.Comment.QuoteOffset).
func appendQuotedCommentScoped(cj *session.HunkyardJSON, filePath string, startLine, endLine int, body, quote string, quoteOffset *int, author string, scope session.InheritedScope) {
	now := time.Now().UTC().Format(time.RFC3339)
	cj.UpdatedAt = now

	cf, ok := cj.Files[filePath]
	if !ok {
		cf = session.HunkyardJSONFile{
			Status:   "modified",
			Comments: []session.Comment{},
		}
	}

	// Populate anchor from the file on disk.
	anchor := readAnchorFromDisk(filePath, startLine, endLine)

	c := session.StampWithFocus(session.Comment{
		AuthorKind:  "agent",
		ID:          session.RandomCommentID(),
		StartLine:   startLine,
		EndLine:     endLine,
		Body:        body,
		Quote:       quote,
		QuoteOffset: quoteOffset,
		Anchor:      anchor,
		Author:      author,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, scope.AsFocus())
	cf.Comments = append(cf.Comments, c)
	cj.Files[filePath] = cf
}

// readAnchorFromDisk reads the file from disk and extracts lines startLine..endLine
// as the anchor text. Returns empty string on any error or if the file is not found.
func readAnchorFromDisk(filePath string, startLine, endLine int) string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	return session.ExtractAnchor(string(data), startLine, endLine)
}

// appendReply adds a reply to an existing comment in the session.HunkyardJSON struct in memory.
// Returns an error if the comment ID is not found or is ambiguous across files.
// Searches both file comments and review_comments.
func appendReply(cj *session.HunkyardJSON, commentID, body, author string, resolve bool, filterPath string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	cj.UpdatedAt = now

	// Check all review comments (not just those starting with "r" — web-fetched ones use "web-N").
	for i, c := range cj.ReviewComments {
		if c.ID == commentID {
			reply := session.Reply{
				AuthorKind:  "agent",
				ID:          session.RandomReplyID(),
				Body:        body,
				Author:      author,
				CreatedAt:   now,
				ReviewRound: cj.ReviewRound,
			}
			cj.ReviewComments[i].Replies = append(cj.ReviewComments[i].Replies, reply)
			cj.ReviewComments[i].UpdatedAt = now
			if resolve {
				cj.ReviewComments[i].Resolved = true
				cj.ReviewComments[i].ResolvedRound = cj.ReviewRound
			} else {
				// Match HTTP AddReviewCommentReply semantics: a non-resolving
				// reply unresolves a previously-resolved comment so the new
				// reply doesn't get hidden by the resolution filter.
				cj.ReviewComments[i].Resolved = false
				cj.ReviewComments[i].ResolvedRound = 0
			}
			return nil
		}
	}

	// Search file comments
	var found bool
	var foundPaths []string
	for filePath, cf := range cj.Files {
		if filterPath != "" && filePath != filterPath {
			continue
		}
		for i, c := range cf.Comments {
			if c.ID == commentID {
				foundPaths = append(foundPaths, filePath)
				if !found {
					found = true
					reply := session.Reply{
						AuthorKind:  "agent",
						ID:          session.RandomReplyID(),
						Body:        body,
						Author:      author,
						CreatedAt:   now,
						ReviewRound: cj.ReviewRound,
					}
					cf.Comments[i].Replies = append(cf.Comments[i].Replies, reply)
					cf.Comments[i].UpdatedAt = now
					if resolve {
						cf.Comments[i].Resolved = true
						cf.Comments[i].ResolvedRound = cj.ReviewRound
					} else {
						// Match HTTP AddReply semantics: a non-resolving reply
						// unresolves a previously-resolved comment so the new
						// reply doesn't get hidden by the resolution filter.
						cf.Comments[i].Resolved = false
						cf.Comments[i].ResolvedRound = 0
					}
					cj.Files[filePath] = cf
				}
			}
		}
	}

	if len(foundPaths) > 1 {
		return fmt.Errorf("comment %q found in multiple files (%s); use --path <file> to disambiguate",
			commentID, strings.Join(foundPaths, ", "))
	}
	if !found {
		if filterPath != "" {
			return fmt.Errorf("comment %q not found in file %q in review file", commentID, filterPath)
		}
		return fmt.Errorf("comment %q not found in review file", commentID)
	}
	return nil
}

// addCommentToHunkyardJSONScoped appends a comment to the review file for the given file and line range.
// Creates the review file if it doesn't exist. Appends to existing comments if it does.
// Works in both git repos and plain directories (file mode).
// outputDir overrides the default location (repo root or CWD) when non-empty.
// scope carries optional HeadSHA / DiffScope stamping; the zero value produces
// working-tree behavior.
func addCommentToHunkyardJSONScoped(filePath string, startLine, endLine int, body, author, outputDir string, scope session.InheritedScope) error {
	hunkyardPath, err := review.ResolveReviewPath(outputDir)
	if err != nil {
		return err
	}
	return addCommentToHunkyardJSONAtPath(filePath, startLine, endLine, body, author, hunkyardPath, scope)
}

func addCommentToHunkyardJSONAtPath(filePath string, startLine, endLine int, body, author, hunkyardPath string, scope session.InheritedScope) error {
	if isAbsoluteOrTraversal(filePath) {
		return fmt.Errorf("path %q must be relative and within the repository", filePath)
	}
	cleaned := filepath.Clean(filePath)
	// Review JSON is a cross-platform artefact (synced via GitHub),
	// so paths are stored with forward slashes regardless of host OS.
	cleaned = filepath.ToSlash(cleaned)

	cj, err := review.LoadHunkyardJSON(hunkyardPath)
	if err != nil {
		return err
	}

	appendCommentScoped(&cj, cleaned, startLine, endLine, body, author, scope)
	return review.SaveHunkyardJSON(hunkyardPath, cj)
}

// addReplyToHunkyardJSON adds a reply to an existing comment in the review file.
// It searches all files for the comment ID. If resolve is true, it also marks the comment as resolved.
func addReplyToHunkyardJSON(commentID, body, author string, resolve bool, outputDir string, filterPath string) error {
	hunkyardPath, err := review.ResolveReviewPath(outputDir)
	if err != nil {
		return err
	}
	return addReplyToHunkyardJSONAtPath(commentID, body, author, resolve, hunkyardPath, filterPath)
}

func addReplyToHunkyardJSONAtPath(commentID, body, author string, resolve bool, hunkyardPath string, filterPath string) error {
	return addReplyToHunkyardJSONAtPathWithRedirect(commentID, body, author, resolve, hunkyardPath, filterPath, true)
}

func addReplyToHunkyardJSONAtPathWithRedirect(commentID, body, author string, resolve bool, hunkyardPath string, filterPath string, allowRedirect bool) error {
	cj, err := review.LoadHunkyardJSON(hunkyardPath)
	if err != nil {
		return err
	}

	if err := appendReply(&cj, commentID, body, author, resolve, filterPath); err != nil {
		// Only fall back to scanning when the comment genuinely wasn't found.
		// Don't fall back for ambiguity errors ("found in multiple files").
		if allowRedirect && strings.Contains(err.Error(), "not found") {
			if altPath, err2 := review.FindReviewFileByCommentID(commentID, hunkyardPath); err2 == nil {
				altCJ, loadErr := review.LoadHunkyardJSON(altPath)
				if loadErr != nil {
					return err // return original error
				}
				if replyErr := appendReply(&altCJ, commentID, body, author, resolve, filterPath); replyErr != nil {
					return err // return original error
				}
				fmt.Fprintf(os.Stderr, "Note: comment %s found in %s (not the resolved review file)\n", commentID, filepath.Base(altPath))
				return review.SaveHunkyardJSON(altPath, altCJ)
			}
		}
		return err
	}
	return review.SaveHunkyardJSON(hunkyardPath, cj)
}

// BulkCommentEntry represents one entry in a bulk comment JSON array.
// Supports review-level, file-level, line-level comments, replies, and
// DOM pins (selector / route) for live and preview reviews.
type BulkCommentEntry struct {
	// New comment fields
	File     string `json:"file,omitempty"`
	Path     string `json:"path,omitempty"`     // alias for File
	Line     int    `json:"-"`                  // parsed from "line" (int or string like "45-47")
	LineSpec string `json:"-"`                  // string line spec like "45-47" (from "line" field)
	EndLine  int    `json:"end_line,omitempty"` // defaults to Line if omitted
	Body     string `json:"body"`
	Author   string `json:"author,omitempty"` // overrides per-entry; falls back to global
	Scope    string `json:"scope,omitempty"`  // "review", "file", or "" (inferred)

	// Pin fields for live and preview reviews. Selector is a CSS selector.
	// CSSSelector and Pathname are aliases so a dom_anchor-shaped payload works.
	Selector    string `json:"selector,omitempty"`
	CSSSelector string `json:"css_selector,omitempty"`
	Route       string `json:"route,omitempty"`
	Pathname    string `json:"pathname,omitempty"`

	// Quote narrows a line comment to the text the reviewer selected, as the
	// web UI's text-selection comments do. QuoteOffset is optional.
	Quote       string `json:"quote,omitempty"`
	QuoteOffset *int   `json:"quote_offset,omitempty"`

	// session.Reply fields
	ReplyTo string `json:"reply_to,omitempty"`
	Resolve bool   `json:"resolve,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling for BulkCommentEntry
// to handle the "line" field being either an int (42) or a string ("45-47").
func (e *BulkCommentEntry) UnmarshalJSON(data []byte) error {
	// Use an alias to avoid infinite recursion
	type Alias BulkCommentEntry
	aux := &struct {
		Line json.RawMessage `json:"line,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(e),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if len(aux.Line) > 0 && string(aux.Line) != "null" {
		// Try int first
		var lineInt int
		if err := json.Unmarshal(aux.Line, &lineInt); err == nil {
			e.Line = lineInt
			return nil
		}
		// Try string
		var lineStr string
		if err := json.Unmarshal(aux.Line, &lineStr); err == nil {
			e.LineSpec = lineStr
			return nil
		}
		return fmt.Errorf("line must be int or string, got %s", aux.Line)
	}
	return nil
}

// processBulkEntry routes a single bulk comment entry to the appropriate
// authoring helper. globalAuthor is used when an entry doesn't specify its own
// author. scope is stamped on every authored comment (empty = today's behavior).
// pages caches live page fetches for pins across the bulk run.
func processBulkEntry(cj *session.HunkyardJSON, i int, e BulkCommentEntry, globalAuthor string, scope session.InheritedScope, pages livePageCache) error {
	if e.Body == "" {
		return fmt.Errorf("entry %d: body is required", i)
	}

	author := e.Author
	if author == "" {
		author = globalAuthor
	}

	if e.ReplyTo != "" {
		return processBulkReply(cj, i, e, author)
	}

	if e.pinSelector() != "" || e.pinRoute() != "" || cj.ReviewType == "live" || cj.ReviewType == "preview" {
		return processBulkPinOrDOMReview(cj, i, e, author, pages)
	}

	if e.Scope == "review" || (e.File == "" && e.Path == "" && e.Line <= 0 && e.LineSpec == "") {
		if hasQuote(e) {
			return fmt.Errorf("entry %d: quote needs a line comment", i)
		}
		return processBulkReviewEntry(cj, i, e, author, scope)
	}

	return processBulkFileOrLineEntry(cj, i, e, author, scope)
}

func processBulkReply(cj *session.HunkyardJSON, i int, e BulkCommentEntry, author string) error {
	if e.pinSelector() != "" || e.pinRoute() != "" {
		return fmt.Errorf("entry %d: selector cannot be combined with reply_to", i)
	}
	if err := appendReply(cj, e.ReplyTo, e.Body, author, e.Resolve, e.File); err != nil {
		return fmt.Errorf("entry %d: %w", i, err)
	}
	return nil
}

func processBulkPinOrDOMReview(cj *session.HunkyardJSON, i int, e BulkCommentEntry, author string, pages livePageCache) error {
	if e.pinSelector() == "" && e.pinRoute() == "" {
		return fmt.Errorf("entry %d: %s is not supported for %s reviews; set \"selector\" on the entry (the JSON form of --selector)", i, bulkEntryForm(e), cj.ReviewType)
	}
	if e.File != "" || e.Path != "" || e.Line > 0 || e.LineSpec != "" {
		return fmt.Errorf("entry %d: selector cannot be combined with a file or line", i)
	}
	if hasQuote(e) {
		return fmt.Errorf("entry %d: quote is not valid on a pin", i)
	}
	if _, err := appendPin(cj, e.pinSelector(), e.pinRoute(), e.Body, author, pages); err != nil {
		return fmt.Errorf("entry %d: %w", i, err)
	}
	return nil
}

func processBulkReviewEntry(cj *session.HunkyardJSON, i int, e BulkCommentEntry, author string, scope session.InheritedScope) error {
	if e.Line > 0 || e.LineSpec != "" {
		return fmt.Errorf("entry %d: file is required for new comments", i)
	}
	if e.Scope != "review" && (e.File != "" || e.Path != "") {
		return fmt.Errorf("entry %d: file is required for new comments", i)
	}
	appendReviewCommentScoped(cj, e.Body, author, scope)
	return nil
}

func processBulkFileOrLineEntry(cj *session.HunkyardJSON, i int, e BulkCommentEntry, author string, scope session.InheritedScope) error {
	filePath := e.File
	if filePath == "" {
		filePath = e.Path
	}
	if filePath == "" {
		return fmt.Errorf("entry %d: file is required for new comments", i)
	}

	// Normalize backslashes before the traversal check — on Unix filepath.Clean
	// treats backslash as a literal character, so "subdir\..\..\etc\passwd" would
	// pass isAbsoluteOrTraversal and only reveal the traversal after conversion.
	normalizedPath := strings.ReplaceAll(filePath, `\`, "/")
	if isAbsoluteOrTraversal(normalizedPath) {
		return fmt.Errorf("entry %d: path %q must be relative and within the repository", i, filePath)
	}
	// Normalize for cross-platform storage — see addCommentToHunkyardJSONScoped.
	cleaned := filepath.ToSlash(filepath.Clean(normalizedPath))

	if e.Scope == "file" || (e.Line <= 0 && e.LineSpec == "" && e.Path != "" && e.File == "") {
		if hasQuote(e) {
			return fmt.Errorf("entry %d: quote needs a line comment", i)
		}
		appendFileCommentScoped(cj, cleaned, e.Body, author, scope)
		return nil
	}

	if e.Line <= 0 && e.LineSpec == "" {
		return fmt.Errorf("entry %d: line must be > 0", i)
	}

	return processBulkLineComment(cj, i, e, cleaned, author, scope)
}

func processBulkLineComment(cj *session.HunkyardJSON, i int, e BulkCommentEntry, cleaned, author string, scope session.InheritedScope) error {
	startLine := e.Line
	endLine := e.EndLine

	if e.LineSpec != "" && startLine == 0 {
		var err error
		startLine, endLine, err = parseLineSpec(e.LineSpec)
		if err != nil {
			return fmt.Errorf("entry %d: invalid line spec %q", i, e.LineSpec)
		}
	}

	if startLine <= 0 {
		return fmt.Errorf("entry %d: line must be > 0", i)
	}
	if endLine == 0 {
		endLine = startLine
	}

	if e.QuoteOffset != nil && (*e.QuoteOffset < 0 || e.Quote == "") {
		return fmt.Errorf("entry %d: quote_offset needs a quote and must be >= 0", i)
	}

	appendQuotedCommentScoped(cj, cleaned, startLine, endLine, e.Body, e.Quote, e.QuoteOffset, author, scope)
	return nil
}

func hasQuote(e BulkCommentEntry) bool {
	return e.Quote != "" || e.QuoteOffset != nil
}

func parseLineSpec(spec string) (start, end int, err error) {
	if dashIdx := strings.Index(spec, "-"); dashIdx >= 0 {
		s, err1 := strconv.Atoi(spec[:dashIdx])
		e, err2 := strconv.Atoi(spec[dashIdx+1:])
		if err1 != nil || err2 != nil {
			if err1 != nil {
				return 0, 0, err1
			}
			return 0, 0, err2
		}
		return s, e, nil
	}
	n, err := strconv.Atoi(spec)
	if err != nil {
		return 0, 0, err
	}
	return n, n, nil
}

// bulkAddCommentsToHunkyardJSONScoped writes a batch of comments to a single review
// file with scope stamping applied to every entry.
//
// Target resolution: a bulk call always writes to a single review file. If any
// entry uses reply_to and none of the referenced IDs live in the cwd-resolved
// primary file, the entire bulk is redirected to the alt file that contains
// them. New comments in the same bulk ride along. If reply IDs split across
// multiple review files (or some land in primary and others elsewhere), the
// call is rejected — callers should split into per-file bulks.
func bulkAddCommentsToHunkyardJSONScoped(entries []BulkCommentEntry, globalAuthor string, outputDir string, scope session.InheritedScope) error {
	primaryPath, err := review.ResolveReviewPath(outputDir)
	if err != nil {
		return err
	}
	return bulkAddCommentsToHunkyardJSONAtPath(entries, globalAuthor, primaryPath, scope)
}

func bulkAddCommentsToHunkyardJSONAtPath(entries []BulkCommentEntry, globalAuthor, primaryPath string, scope session.InheritedScope) error {
	return bulkAddCommentsToHunkyardJSONAtPathWithRedirect(entries, globalAuthor, primaryPath, scope, true)
}

func bulkAddCommentsToHunkyardJSONAtPathWithRedirect(entries []BulkCommentEntry, globalAuthor, primaryPath string, scope session.InheritedScope, allowRedirect bool) error {
	if len(entries) == 0 {
		return fmt.Errorf("no comment entries provided")
	}
	primary, err := review.LoadHunkyardJSON(primaryPath)
	if err != nil {
		return err
	}

	targetPath, targetCJ, redirected, err := resolveBulkTargetWithRedirect(entries, primaryPath, primary, allowRedirect)
	if err != nil {
		return err
	}

	pages := livePageCache{}
	for i, e := range entries {
		if err := processBulkEntry(&targetCJ, i, e, globalAuthor, scope, pages); err != nil {
			return err
		}
	}

	if err := review.SaveHunkyardJSON(targetPath, targetCJ); err != nil {
		return err
	}
	if redirected {
		fmt.Fprintf(os.Stderr, "Note: replies routed to %s (not the cwd-resolved review file)\n", filepath.Base(targetPath))
	}
	return nil
}

// resolveBulkTarget picks the single review file that this bulk call should
// write to. Returns the path, the loaded session.HunkyardJSON to mutate, and whether the
// target differs from the cwd-resolved primary.
func resolveBulkTarget(entries []BulkCommentEntry, primaryPath string, primary session.HunkyardJSON) (string, session.HunkyardJSON, bool, error) {
	return resolveBulkTargetWithRedirect(entries, primaryPath, primary, true)
}

func resolveBulkTargetWithRedirect(entries []BulkCommentEntry, primaryPath string, primary session.HunkyardJSON, allowRedirect bool) (string, session.HunkyardJSON, bool, error) {
	var replyIDs []string
	seen := map[string]bool{}
	for _, e := range entries {
		if e.ReplyTo == "" || seen[e.ReplyTo] {
			continue
		}
		seen[e.ReplyTo] = true
		replyIDs = append(replyIDs, e.ReplyTo)
	}

	if len(replyIDs) == 0 {
		return primaryPath, primary, false, nil
	}

	var inPrimary, missing []string
	for _, id := range replyIDs {
		if review.CJContainsCommentID(&primary, id) {
			inPrimary = append(inPrimary, id)
		} else {
			missing = append(missing, id)
		}
	}

	if len(missing) == 0 {
		return primaryPath, primary, false, nil
	}
	if len(inPrimary) > 0 {
		return "", session.HunkyardJSON{}, false, fmt.Errorf(
			"bulk targets multiple review files: %v exist in %s, but %v do not — split into per-file bulks",
			inPrimary, filepath.Base(primaryPath), missing,
		)
	}
	if !allowRedirect {
		return "", session.HunkyardJSON{}, false, fmt.Errorf("reply targets %v not found in selected review file", missing)
	}

	// None in primary — every reply ID must live in the same alt file.
	var altPath string
	for _, id := range missing {
		path, err := review.FindReviewFileByCommentID(id, primaryPath)
		if err != nil {
			return "", session.HunkyardJSON{}, false, fmt.Errorf("reply target %s: %w", id, err)
		}
		if altPath == "" {
			altPath = path
			continue
		}
		if path != altPath {
			return "", session.HunkyardJSON{}, false, fmt.Errorf(
				"bulk targets multiple review files: %s in %s, %s in %s — split into per-file bulks",
				missing[0], filepath.Base(altPath), id, filepath.Base(path),
			)
		}
	}

	altCJ, err := review.LoadHunkyardJSON(altPath)
	if err != nil {
		return "", session.HunkyardJSON{}, false, fmt.Errorf("load %s: %w", filepath.Base(altPath), err)
	}
	return altPath, altCJ, true, nil
}

// addReviewCommentToHunkyardJSONScoped adds a review-level comment to the review
// file with scope stamping.
func addReviewCommentToHunkyardJSONScoped(body, author, outputDir string, scope session.InheritedScope) error {
	hunkyardPath, err := review.ResolveReviewPath(outputDir)
	if err != nil {
		return err
	}
	return addReviewCommentToHunkyardJSONAtPath(body, author, hunkyardPath, scope)
}

func addReviewCommentToHunkyardJSONAtPath(body, author, hunkyardPath string, scope session.InheritedScope) error {
	cj, err := review.LoadHunkyardJSON(hunkyardPath)
	if err != nil {
		return err
	}

	appendReviewCommentScoped(&cj, body, author, scope)
	return review.SaveHunkyardJSON(hunkyardPath, cj)
}

// addFileCommentToHunkyardJSONScoped adds a file-level comment to the review file
// with scope stamping.
func addFileCommentToHunkyardJSONScoped(filePath, body, author, outputDir string, scope session.InheritedScope) error {
	hunkyardPath, err := review.ResolveReviewPath(outputDir)
	if err != nil {
		return err
	}
	return addFileCommentToHunkyardJSONAtPath(filePath, body, author, hunkyardPath, scope)
}

func addFileCommentToHunkyardJSONAtPath(filePath, body, author, hunkyardPath string, scope session.InheritedScope) error {
	if isAbsoluteOrTraversal(filePath) {
		return fmt.Errorf("path %q must be relative and within the repository", filePath)
	}
	cleaned := filepath.Clean(filePath)
	// Review JSON is a cross-platform artefact (synced via GitHub),
	// so paths are stored with forward slashes regardless of host OS.
	cleaned = filepath.ToSlash(cleaned)

	cj, err := review.LoadHunkyardJSON(hunkyardPath)
	if err != nil {
		return err
	}

	appendFileCommentScoped(&cj, cleaned, body, author, scope)
	return review.SaveHunkyardJSON(hunkyardPath, cj)
}

// appendReviewCommentScoped adds a review-level comment to the session.HunkyardJSON struct
// in memory, stamping DiffScope/HeadSHA from scope.
func appendReviewCommentScoped(cj *session.HunkyardJSON, body, author string, scope session.InheritedScope) {
	now := time.Now().UTC().Format(time.RFC3339)
	cj.UpdatedAt = now

	c := session.StampWithFocus(session.Comment{
		AuthorKind: "agent",
		ID:         session.RandomReviewCommentID(),
		Body:       body,
		Author:     author,
		Scope:      "review",
		CreatedAt:  now,
		UpdatedAt:  now,
	}, scope.AsFocus())
	cj.ReviewComments = append(cj.ReviewComments, c)
}

// appendFileCommentScoped adds a file-level comment (scope: "file", lines: 0)
// to the session.HunkyardJSON struct in memory, stamping DiffScope/HeadSHA from scope.
func appendFileCommentScoped(cj *session.HunkyardJSON, filePath, body, author string, scope session.InheritedScope) {
	now := time.Now().UTC().Format(time.RFC3339)
	cj.UpdatedAt = now

	cf, ok := cj.Files[filePath]
	if !ok {
		cf = session.HunkyardJSONFile{
			Status:   "modified",
			Comments: []session.Comment{},
		}
	}

	c := session.StampWithFocus(session.Comment{
		AuthorKind: "agent",
		ID:         session.RandomCommentID(),
		Body:       body,
		Author:     author,
		Scope:      "file",
		CreatedAt:  now,
		UpdatedAt:  now,
	}, scope.AsFocus())
	cf.Comments = append(cf.Comments, c)
	cj.Files[filePath] = cf
}
