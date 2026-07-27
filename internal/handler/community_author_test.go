package handler

import (
	"testing"

	"github.com/strelov1/freehire/internal/community"
)

// Content outlives its author: a deleted account leaves the persona gone and the
// handle empty. That must read as a departed member, never as the AI persona — the
// two are different speakers and conflating them puts a person's words in a bot's
// mouth.
func TestDeAuthoredContentIsNotTheAIPersona(t *testing.T) {
	t.Run("reply by a deleted member", func(t *testing.T) {
		got := toReplyResponse(community.Reply{ID: 1, ThreadID: 2, AuthorHandle: "", IsAI: false})
		if got.Author == aiAuthor {
			t.Errorf("author = %q, want a deleted-member marker, not the AI persona", got.Author)
		}
		if got.Author == "" {
			t.Error("author = \"\", want an explicit deleted-member marker")
		}
	})

	t.Run("AI reply keeps the AI persona", func(t *testing.T) {
		got := toReplyResponse(community.Reply{ID: 1, ThreadID: 2, AuthorHandle: "", IsAI: true})
		if got.Author != aiAuthor {
			t.Errorf("author = %q, want %q", got.Author, aiAuthor)
		}
	})

	t.Run("live handle is untouched", func(t *testing.T) {
		got := toReplyResponse(community.Reply{ID: 1, ThreadID: 2, AuthorHandle: "quiet-otter"})
		if got.Author != "quiet-otter" {
			t.Errorf("author = %q, want the persona handle", got.Author)
		}
	})

	t.Run("thread by a deleted member", func(t *testing.T) {
		got := toThreadResponse(community.Thread{ID: 1, AuthorHandle: ""})
		if got.Author == "" || got.Author == aiAuthor {
			t.Errorf("author = %q, want an explicit deleted-member marker", got.Author)
		}
	})
}
