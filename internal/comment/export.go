package comment

import "github.com/BSH2409/hunkyard/internal/session"

func CheckCommentCLIAllowed(hunkyardPath, form string) error {
	return checkCommentCLIAllowed(hunkyardPath, form)
}

func AddCommentToHunkyardJSONScoped(filePath string, startLine, endLine int, body, author, outputDir string, scope session.InheritedScope) error {
	return addCommentToHunkyardJSONScoped(filePath, startLine, endLine, body, author, outputDir, scope)
}

func AddReplyToHunkyardJSON(commentID, body, author string, resolve bool, outputDir, filterPath string) error {
	return addReplyToHunkyardJSON(commentID, body, author, resolve, outputDir, filterPath)
}

func AppendReply(cj *session.HunkyardJSON, commentID, body, author string, resolve bool, filterPath string) error {
	return appendReply(cj, commentID, body, author, resolve, filterPath)
}

func BulkAddCommentsToHunkyardJSONScoped(entries []BulkCommentEntry, globalAuthor, outputDir string, scope session.InheritedScope) error {
	return bulkAddCommentsToHunkyardJSONScoped(entries, globalAuthor, outputDir, scope)
}

func AddReviewCommentToHunkyardJSONScoped(body, author, outputDir string, scope session.InheritedScope) error {
	return addReviewCommentToHunkyardJSONScoped(body, author, outputDir, scope)
}

func AddFileCommentToHunkyardJSONScoped(filePath, body, author, outputDir string, scope session.InheritedScope) error {
	return addFileCommentToHunkyardJSONScoped(filePath, body, author, outputDir, scope)
}

func AppendFileCommentScoped(cj *session.HunkyardJSON, filePath, body, author string, scope session.InheritedScope) {
	appendFileCommentScoped(cj, filePath, body, author, scope)
}

func AppendReviewCommentScoped(cj *session.HunkyardJSON, body, author string, scope session.InheritedScope) {
	appendReviewCommentScoped(cj, body, author, scope)
}

func AppendCommentScoped(cj *session.HunkyardJSON, filePath string, startLine, endLine int, body, author string, scope session.InheritedScope) {
	appendCommentScoped(cj, filePath, startLine, endLine, body, author, scope)
}

func ProcessBulkReviewEntry(cj *session.HunkyardJSON, i int, e BulkCommentEntry, author string, scope session.InheritedScope) error {
	return processBulkReviewEntry(cj, i, e, author, scope)
}

// ParseLineSpec parses a line range spec like "10" or "10-20".
func ParseLineSpec(spec string) (start, end int, err error) {
	return parseLineSpec(spec)
}

// ListCommentsFromHunkyardJSON returns comments from HunkyardJSON in agent-friendly order.
func ListCommentsFromHunkyardJSON(cj HunkyardJSON, unresolvedOnly bool) []ListedComment {
	return listCommentsFromHunkyardJSON(cj, unresolvedOnly)
}

// FormatCommentsText renders listed comments as human-readable text.
func FormatCommentsText(entries []ListedComment, unresolvedOnly bool) string {
	return formatCommentsText(entries, unresolvedOnly)
}
