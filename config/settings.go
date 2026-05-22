package config

import (
	"fmt"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

// Settings 是用户可见且可在三层配置中持久化的字段。
// 指针类型用于区分 "未设置"（继承上层）与 "显式置零"。
type Settings struct {
	// 模型
	OpenAIAPIKey  string `toml:"openai_api_key,omitempty" json:"openai_api_key,omitempty"`
	OpenAIBaseURL string `toml:"openai_base_url,omitempty" json:"openai_base_url,omitempty"`
	OpenAIModel   string `toml:"openai_model,omitempty" json:"openai_model,omitempty"`

	// 路径
	SkillsDir string `toml:"skills_dir,omitempty" json:"skills_dir,omitempty"`
	NovaDir   string `toml:"nova_dir,omitempty" json:"nova_dir,omitempty"`

	// 编辑器
	AutoSaveEnabled       *bool  `toml:"auto_save_enabled,omitempty" json:"auto_save_enabled,omitempty"`
	AutoSaveIntervalMs    *int   `toml:"auto_save_interval_ms,omitempty" json:"auto_save_interval_ms,omitempty"`
	ChapterFilenameFormat string `toml:"chapter_filename_format,omitempty" json:"chapter_filename_format,omitempty"`

	// Agent
	MaxIteration    *int  `toml:"max_iteration,omitempty" json:"max_iteration,omitempty"`
	ModelMaxRetries *int  `toml:"model_max_retries,omitempty" json:"model_max_retries,omitempty"`
	PlanModeDefault *bool `toml:"plan_mode_default,omitempty" json:"plan_mode_default,omitempty"`
}

func boolPtr(v bool) *bool { return &v }
func intPtr(v int) *int    { return &v }

// DefaultSettings 返回内置默认配置（最低优先级）。
func DefaultSettings() Settings {
	return Settings{
		OpenAIBaseURL:         "https://api.deepseek.com",
		OpenAIModel:           "deepseek-v4-pro",
		SkillsDir:             "./skills",
		NovaDir:               "~/.nova",
		AutoSaveEnabled:       boolPtr(true),
		AutoSaveIntervalMs:    intPtr(1500),
		ChapterFilenameFormat: "ch{NN}-{title}.md",
		MaxIteration:          intPtr(50),
		ModelMaxRetries:       intPtr(5),
		PlanModeDefault:       boolPtr(false),
	}
}

// Merge 用 child 的非零字段覆盖 parent 后返回新值。
// 字符串：空串视为未设置；指针：nil 视为未设置。
func Merge(parent, child Settings) Settings {
	out := parent
	if child.OpenAIAPIKey != "" {
		out.OpenAIAPIKey = child.OpenAIAPIKey
	}
	if child.OpenAIBaseURL != "" {
		out.OpenAIBaseURL = child.OpenAIBaseURL
	}
	if child.OpenAIModel != "" {
		out.OpenAIModel = child.OpenAIModel
	}
	if child.SkillsDir != "" {
		out.SkillsDir = child.SkillsDir
	}
	if child.NovaDir != "" {
		out.NovaDir = child.NovaDir
	}
	if child.AutoSaveEnabled != nil {
		out.AutoSaveEnabled = child.AutoSaveEnabled
	}
	if child.AutoSaveIntervalMs != nil {
		out.AutoSaveIntervalMs = child.AutoSaveIntervalMs
	}
	if child.ChapterFilenameFormat != "" {
		out.ChapterFilenameFormat = child.ChapterFilenameFormat
	}
	if child.MaxIteration != nil {
		out.MaxIteration = child.MaxIteration
	}
	if child.ModelMaxRetries != nil {
		out.ModelMaxRetries = child.ModelMaxRetries
	}
	if child.PlanModeDefault != nil {
		out.PlanModeDefault = child.PlanModeDefault
	}
	return out
}

const (
	// UserConfigFilename 是用户级配置文件名（位于 NovaDir 下）。
	UserConfigFilename = "config.toml"
	// WorkspaceConfigDir 是工作区级配置目录（相对于 workspace）。
	WorkspaceConfigDir = ".nova"
	// WorkspaceConfigFilename 是工作区级配置文件名。
	WorkspaceConfigFilename = "config.toml"
)

// LayeredSettings 暴露三层快照及合并后的 effective 值。
type LayeredSettings struct {
	Default   Settings `json:"default"`
	User      Settings `json:"user"`
	Workspace Settings `json:"workspace"`
	Effective Settings `json:"effective"`
}

// ReadSettingsFile 读取 TOML，文件不存在时返回零值且无错误。
func ReadSettingsFile(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Settings{}, nil
		}
		return Settings{}, fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	var s Settings
	if err := toml.Unmarshal(data, &s); err != nil {
		return Settings{}, fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	return s, nil
}

// WriteSettingsFile 写入 TOML，自动创建父目录。
func WriteSettingsFile(path string, s Settings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	data, err := toml.Marshal(s)
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", path, err)
	}
	return nil
}

// UserConfigPath 计算用户级配置路径。novaDir 已经过 normalizePath 处理。
func UserConfigPath(novaDir string) string {
	if novaDir == "" {
		novaDir = normalizePath("~/.nova")
	}
	return filepath.Join(novaDir, UserConfigFilename)
}

// WorkspaceConfigPath 计算工作区级配置路径。
func WorkspaceConfigPath(workspace string) string {
	return filepath.Join(workspace, WorkspaceConfigDir, WorkspaceConfigFilename)
}

// LoadLayered 读取用户级 + 工作区级配置并与默认值合并。
// novaDir 为空时使用默认 ~/.nova。
func LoadLayered(novaDir, workspace string) (LayeredSettings, error) {
	user, err := ReadSettingsFile(UserConfigPath(novaDir))
	if err != nil {
		return LayeredSettings{}, err
	}
	var ws Settings
	if workspace != "" {
		ws, err = ReadSettingsFile(WorkspaceConfigPath(workspace))
		if err != nil {
			return LayeredSettings{}, err
		}
	}
	def := DefaultSettings()
	eff := Merge(Merge(def, user), ws)
	return LayeredSettings{Default: def, User: user, Workspace: ws, Effective: eff}, nil
}
