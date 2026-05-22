package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSettingsValues(t *testing.T) {
	s := DefaultSettings()
	if s.OpenAIBaseURL != "https://api.deepseek.com" {
		t.Fatalf("BaseURL: %s", s.OpenAIBaseURL)
	}
	if s.OpenAIModel != "deepseek-v4-pro" {
		t.Fatalf("Model: %s", s.OpenAIModel)
	}
	if s.AutoSaveEnabled == nil || *s.AutoSaveEnabled != true {
		t.Fatalf("AutoSaveEnabled default")
	}
	if s.MaxIteration == nil || *s.MaxIteration != 50 {
		t.Fatalf("MaxIteration default")
	}
}

func TestMergeOverridesNonZero(t *testing.T) {
	parent := Settings{
		OpenAIBaseURL: "https://parent",
		OpenAIModel:   "p-model",
		MaxIteration:  intPtr(10),
	}
	child := Settings{
		OpenAIModel:  "c-model", // override
		MaxIteration: nil,       // 继承 parent
	}
	out := Merge(parent, child)
	if out.OpenAIBaseURL != "https://parent" {
		t.Fatalf("BaseURL should inherit: %s", out.OpenAIBaseURL)
	}
	if out.OpenAIModel != "c-model" {
		t.Fatalf("Model should override: %s", out.OpenAIModel)
	}
	if out.MaxIteration == nil || *out.MaxIteration != 10 {
		t.Fatalf("MaxIteration should inherit parent")
	}
}

func TestMergePointerExplicitOverride(t *testing.T) {
	parent := Settings{AutoSaveEnabled: boolPtr(true)}
	child := Settings{AutoSaveEnabled: boolPtr(false)}
	out := Merge(parent, child)
	if out.AutoSaveEnabled == nil || *out.AutoSaveEnabled != false {
		t.Fatalf("explicit false should override true")
	}
}

func TestReadSettingsFileMissingReturnsZero(t *testing.T) {
	s, err := ReadSettingsFile(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if s.OpenAIModel != "" {
		t.Fatalf("missing file should yield zero value")
	}
}

func TestWriteThenReadSettings(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	in := Settings{OpenAIModel: "abc", AutoSaveEnabled: boolPtr(false)}
	if err := WriteSettingsFile(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadSettingsFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.OpenAIModel != "abc" {
		t.Fatalf("model")
	}
	if out.AutoSaveEnabled == nil || *out.AutoSaveEnabled != false {
		t.Fatalf("auto save")
	}
}

func TestLoadLayeredAppliesAllLayers(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".nova"), 0o755); err != nil {
		t.Fatal(err)
	}

	user := Settings{OpenAIModel: "user-model", MaxIteration: intPtr(20)}
	wsCfg := Settings{OpenAIModel: "ws-model"}
	if err := WriteSettingsFile(filepath.Join(home, "config.toml"), user); err != nil {
		t.Fatal(err)
	}
	if err := WriteSettingsFile(filepath.Join(ws, ".nova", "config.toml"), wsCfg); err != nil {
		t.Fatal(err)
	}

	layered, err := LoadLayered(home, ws)
	if err != nil {
		t.Fatal(err)
	}
	if layered.Effective.OpenAIModel != "ws-model" {
		t.Fatalf("workspace should win: %s", layered.Effective.OpenAIModel)
	}
	if layered.Effective.MaxIteration == nil || *layered.Effective.MaxIteration != 20 {
		t.Fatalf("user MaxIteration should inherit: %v", layered.Effective.MaxIteration)
	}
	if layered.User.OpenAIModel != "user-model" {
		t.Fatalf("raw user should be preserved")
	}
}
