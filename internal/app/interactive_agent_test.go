package app

import (
	"testing"

	"github.com/cloudwego/eino/schema"

	"nova/internal/interactive"
)

func TestInteractiveConversationBuildsHistoryAndPersistsAssistantToStory(t *testing.T) {
	store := interactive.NewStore(t.TempDir())
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:         "末日开端",
		Origin:        "主角醒来发现世界已末日",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		User:      "我推开酒馆的门",
		Narrative: "门后传来低沉的风声。",
	}); err != nil {
		t.Fatal(err)
	}

	conversation := newInteractiveConversation(store, story.ID, "", "我点燃火把")
	history, err := conversation.PrepareMessages("我点燃火把", "我点燃火把\n\n请基于互动故事继续。")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 {
		t.Fatalf("history length = %d, want 3", len(history))
	}
	if history[0].Role != schema.User || history[0].Content != "我推开酒馆的门" {
		t.Fatalf("history[0] mismatch: %#v", history[0])
	}
	if history[1].Role != schema.Assistant || history[1].Content != "门后传来低沉的风声。" {
		t.Fatalf("history[1] mismatch: %#v", history[1])
	}
	if history[2].Role != schema.User || history[2].Content != "我点燃火把\n\n请基于互动故事继续。" {
		t.Fatalf("history[2] mismatch: %#v", history[2])
	}

	if err := conversation.AppendAssistant("火光照亮了墙上的新线索。"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 2 {
		t.Fatalf("turn count = %d, want 2", len(snapshot.Turns))
	}
	last := snapshot.Turns[1]
	if last.User != "我点燃火把" || last.Narrative != "火光照亮了墙上的新线索。" {
		t.Fatalf("last turn mismatch: %#v", last)
	}
}
