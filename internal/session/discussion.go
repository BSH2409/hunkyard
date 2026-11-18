package session

import "time"

func (s *Session) AddAgentReviewReply(commentID, replyID, body, author string) (Reply, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.reviewComments {
		comment := &s.reviewComments[index]
		if comment.ID != commentID {
			continue
		}
		for _, reply := range comment.Replies {
			if reply.ID == replyID {
				return reply, true
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		reply := Reply{ID: replyID, Body: body, Author: author, AuthorKind: "agent", CreatedAt: now, ReviewRound: s.ReviewRound}
		comment.Replies = append(comment.Replies, reply)
		comment.UpdatedAt = now
		s.scheduleWrite()
		return reply, true
	}
	return Reply{}, false
}
