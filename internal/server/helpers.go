package server

import (
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/config"
	"github.com/BSH2409/hunkyard/internal/session"
)

func saveAttachment(reviewPath string, data []byte) (string, error) {
	return session.SaveAttachment(reviewPath, data)
}

func attachmentPathFor(reviewPath, filename string) (path, mime string, err error) {
	return session.AttachmentPathFor(reviewPath, filename)
}

func sanitizeAttachmentAltText(name string) string { return session.SanitizeAttachmentAltText(name) }

func commentsAtOrBeforeRound(comments []Comment, round int) []Comment {
	return session.CommentsAtOrBeforeRound(comments, round)
}

func recordSessionStats(sess *Session, author string, startedAt time.Time) {
	session.RecordSessionStats(sess, author, startedAt)
}

func filterPathsIgnored(paths []string, patterns []string) []string {
	return config.FilterPathsIgnored(paths, patterns)
}

func splitLines(content string) []string {
	if content == "" {
		return []string{}
	}
	return strings.Split(content, "\n")
}
