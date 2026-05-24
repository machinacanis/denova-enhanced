package app

import (
	"fmt"
	"log"

	"github.com/cloudwego/eino/schema"

	"nova/internal/interactive"
	"nova/internal/session"
)

type interactiveConversation struct {
	store    *interactive.Store
	storyID  string
	branchID string
	user     string
}

func newInteractiveConversation(store *interactive.Store, storyID, branchID, user string) *interactiveConversation {
	return &interactiveConversation{store: store, storyID: storyID, branchID: branchID, user: user}
}

func (c *interactiveConversation) PrepareMessages(originalMessage, agentMessage string) ([]*schema.Message, error) {
	if c == nil || c.store == nil {
		return nil, fmt.Errorf("互动故事不存在")
	}
	snapshot, err := c.store.Snapshot(c.storyID, c.branchID)
	if err != nil {
		return nil, err
	}
	history := make([]*schema.Message, 0, len(snapshot.Turns)*2+1)
	for _, turn := range snapshot.Turns {
		history = append(history, schema.UserMessage(turn.User))
		history = append(history, schema.AssistantMessage(turn.Narrative, nil))
	}
	history = append(history, schema.UserMessage(agentMessage))
	return history, nil
}

func (c *interactiveConversation) AppendAssistant(content string) error {
	if c == nil || c.store == nil {
		return fmt.Errorf("互动故事不存在")
	}
	_, err := c.store.AppendTurn(c.storyID, interactive.AppendTurnRequest{
		BranchID:  c.branchID,
		User:      c.user,
		Narrative: content,
	})
	return err
}

func (c *interactiveConversation) MarkInterrupted(userMessage, assistantContent, reason string) error {
	log.Printf("[interactive-agent] interruption ignored story_id=%s branch_id=%s reason=%s", c.storyID, c.branchID, reason)
	return nil
}

func (c *interactiveConversation) PendingInterruption() *session.Interruption {
	return nil
}

func (c *interactiveConversation) ResolveInterruption(id string) error {
	return nil
}
