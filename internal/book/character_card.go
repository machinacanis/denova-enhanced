package book

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const charactersFilePath = "setting/characters.md"

var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// CharacterCardImportResult 描述酒馆角色卡导入结果。
type CharacterCardImportResult struct {
	Name       string `json:"name"`
	TargetPath string `json:"target_path"`
	EntryCount int    `json:"entry_count"`
	Message    string `json:"message"`
}

type tavernCard struct {
	Spec                    string               `json:"spec"`
	SpecVersion             string               `json:"spec_version"`
	Name                    string               `json:"name"`
	Description             string               `json:"description"`
	Personality             string               `json:"personality"`
	Scenario                string               `json:"scenario"`
	FirstMes                string               `json:"first_mes"`
	MesExample              string               `json:"mes_example"`
	CreatorNotes            string               `json:"creator_notes"`
	SystemPrompt            string               `json:"system_prompt"`
	PostHistoryInstructions string               `json:"post_history_instructions"`
	Tags                    []string             `json:"tags"`
	AlternateGreetings      []string             `json:"alternate_greetings"`
	CharacterBook           *tavernCharacterBook `json:"character_book"`
	Data                    *tavernCardData      `json:"data"`
}

type tavernCardData struct {
	Name                    string               `json:"name"`
	Description             string               `json:"description"`
	Personality             string               `json:"personality"`
	Scenario                string               `json:"scenario"`
	FirstMes                string               `json:"first_mes"`
	MesExample              string               `json:"mes_example"`
	CreatorNotes            string               `json:"creator_notes"`
	SystemPrompt            string               `json:"system_prompt"`
	PostHistoryInstructions string               `json:"post_history_instructions"`
	Tags                    []string             `json:"tags"`
	AlternateGreetings      []string             `json:"alternate_greetings"`
	CharacterBook           *tavernCharacterBook `json:"character_book"`
}

type tavernCharacterBook struct {
	Name    string            `json:"name"`
	Entries []tavernBookEntry `json:"entries"`
}

type tavernBookEntry struct {
	ID             int      `json:"id"`
	Keys           []string `json:"keys"`
	SecondaryKeys  []string `json:"secondary_keys"`
	Comment        string   `json:"comment"`
	Content        string   `json:"content"`
	Constant       bool     `json:"constant"`
	Selective      bool     `json:"selective"`
	Enabled        *bool    `json:"enabled"`
	Position       any      `json:"position"`
	InsertionOrder int      `json:"insertion_order"`
}

type normalizedTavernCard struct {
	Spec                    string
	SpecVersion             string
	Name                    string
	Description             string
	Personality             string
	Scenario                string
	FirstMes                string
	MesExample              string
	CreatorNotes            string
	SystemPrompt            string
	PostHistoryInstructions string
	Tags                    []string
	AlternateGreetings      []string
	CharacterBook           *tavernCharacterBook
}

type pngTextChunk struct {
	Keyword string
	Text    string
}

// ImportTavernCharacterCard 将 SillyTavern 酒馆角色卡（PNG 或 JSON）转换为 Markdown，并追加到 setting/characters.md。
func (s *Service) ImportTavernCharacterCard(filename string, data []byte) (CharacterCardImportResult, error) {
	card, err := parseTavernCharacterCard(filename, data)
	if err != nil {
		return CharacterCardImportResult{}, err
	}
	content := renderTavernCardMarkdown(card, filename, time.Now())
	if err := s.appendCharactersMarkdown(content); err != nil {
		return CharacterCardImportResult{}, err
	}

	result := CharacterCardImportResult{
		Name:       card.Name,
		TargetPath: charactersFilePath,
		EntryCount: characterBookEntryCount(card.CharacterBook),
		Message:    fmt.Sprintf("已导入酒馆角色卡「%s」到 %s", card.Name, charactersFilePath),
	}
	return result, nil
}

func parseTavernCharacterCard(filename string, data []byte) (normalizedTavernCard, error) {
	if len(data) == 0 {
		return normalizedTavernCard{}, errors.New("角色卡文件为空")
	}

	var rawJSON []byte
	ext := strings.ToLower(filepath.Ext(filename))
	switch {
	case bytes.HasPrefix(data, pngSignature) || ext == ".png":
		payload, err := extractTavernPayloadFromPNG(data)
		if err != nil {
			return normalizedTavernCard{}, err
		}
		rawJSON = payload
	case ext == ".json" || bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")):
		rawJSON = bytes.TrimSpace(data)
	default:
		return normalizedTavernCard{}, errors.New("仅支持导入 PNG 或 JSON 格式的酒馆角色卡")
	}

	card, err := decodeTavernCardJSON(rawJSON)
	if err != nil {
		return normalizedTavernCard{}, err
	}
	if strings.TrimSpace(card.Name) == "" {
		return normalizedTavernCard{}, errors.New("角色卡缺少 name 字段")
	}
	return card, nil
}

func extractTavernPayloadFromPNG(data []byte) ([]byte, error) {
	chunks, err := extractPNGTextChunks(data)
	if err != nil {
		return nil, err
	}
	for _, chunk := range chunks {
		if chunk.Keyword != "chara" {
			continue
		}
		payload, err := decodeTavernTextPayload(chunk.Text)
		if err != nil {
			return nil, fmt.Errorf("解析 PNG 角色卡元数据失败: %w", err)
		}
		return payload, nil
	}
	return nil, errors.New("PNG 中未找到酒馆角色卡 chara 元数据")
}

func extractPNGTextChunks(data []byte) ([]pngTextChunk, error) {
	if !bytes.HasPrefix(data, pngSignature) {
		return nil, errors.New("不是有效的 PNG 文件")
	}
	var chunks []pngTextChunk
	offset := len(pngSignature)
	for offset+12 <= len(data) {
		length := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		chunkType := string(data[offset+4 : offset+8])
		offset += 8
		if length < 0 || offset+length+4 > len(data) {
			return nil, errors.New("PNG 数据块长度不合法")
		}
		chunkData := data[offset : offset+length]
		offset += length + 4

		switch chunkType {
		case "tEXt":
			chunk, ok := parsePNGTextChunk(chunkData)
			if ok {
				chunks = append(chunks, chunk)
			}
		case "zTXt":
			chunk, err := parsePNGCompressedTextChunk(chunkData)
			if err != nil {
				return nil, err
			}
			if chunk.Keyword != "" {
				chunks = append(chunks, chunk)
			}
		case "iTXt":
			chunk, err := parsePNGInternationalTextChunk(chunkData)
			if err != nil {
				return nil, err
			}
			if chunk.Keyword != "" {
				chunks = append(chunks, chunk)
			}
		case "IEND":
			return chunks, nil
		}
	}
	return chunks, nil
}

func parsePNGTextChunk(data []byte) (pngTextChunk, bool) {
	idx := bytes.IndexByte(data, 0)
	if idx <= 0 {
		return pngTextChunk{}, false
	}
	return pngTextChunk{
		Keyword: string(data[:idx]),
		Text:    string(data[idx+1:]),
	}, true
}

func parsePNGCompressedTextChunk(data []byte) (pngTextChunk, error) {
	idx := bytes.IndexByte(data, 0)
	if idx <= 0 || idx+2 > len(data) {
		return pngTextChunk{}, nil
	}
	if data[idx+1] != 0 {
		return pngTextChunk{}, errors.New("PNG zTXt 使用了不支持的压缩方法")
	}
	text, err := inflateZlib(data[idx+2:])
	if err != nil {
		return pngTextChunk{}, err
	}
	return pngTextChunk{Keyword: string(data[:idx]), Text: text}, nil
}

func parsePNGInternationalTextChunk(data []byte) (pngTextChunk, error) {
	keywordEnd := bytes.IndexByte(data, 0)
	if keywordEnd <= 0 || keywordEnd+3 > len(data) {
		return pngTextChunk{}, nil
	}
	keyword := string(data[:keywordEnd])
	compressionFlag := data[keywordEnd+1]
	compressionMethod := data[keywordEnd+2]
	if compressionMethod != 0 {
		return pngTextChunk{}, errors.New("PNG iTXt 使用了不支持的压缩方法")
	}
	rest := data[keywordEnd+3:]
	languageEnd := bytes.IndexByte(rest, 0)
	if languageEnd < 0 {
		return pngTextChunk{}, nil
	}
	rest = rest[languageEnd+1:]
	translatedEnd := bytes.IndexByte(rest, 0)
	if translatedEnd < 0 {
		return pngTextChunk{}, nil
	}
	textBytes := rest[translatedEnd+1:]
	if compressionFlag == 1 {
		text, err := inflateZlib(textBytes)
		if err != nil {
			return pngTextChunk{}, err
		}
		return pngTextChunk{Keyword: keyword, Text: text}, nil
	}
	return pngTextChunk{Keyword: keyword, Text: string(textBytes)}, nil
}

func inflateZlib(data []byte) (string, error) {
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("解压 PNG 文本块失败: %w", err)
	}
	defer reader.Close()
	out, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("读取 PNG 文本块失败: %w", err)
	}
	return string(out), nil
}

func decodeTavernTextPayload(text string) ([]byte, error) {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "{") {
		return []byte(trimmed), nil
	}
	compacted := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, trimmed)
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	var lastErr error
	for _, enc := range encodings {
		decoded, err := enc.DecodeString(compacted)
		if err == nil {
			return bytes.TrimSpace(decoded), nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func decodeTavernCardJSON(data []byte) (normalizedTavernCard, error) {
	var raw tavernCard
	if err := json.Unmarshal(data, &raw); err != nil {
		return normalizedTavernCard{}, fmt.Errorf("解析角色卡 JSON 失败: %w", err)
	}

	card := normalizedTavernCard{
		Spec:                    raw.Spec,
		SpecVersion:             raw.SpecVersion,
		Name:                    raw.Name,
		Description:             raw.Description,
		Personality:             raw.Personality,
		Scenario:                raw.Scenario,
		FirstMes:                raw.FirstMes,
		MesExample:              raw.MesExample,
		CreatorNotes:            raw.CreatorNotes,
		SystemPrompt:            raw.SystemPrompt,
		PostHistoryInstructions: raw.PostHistoryInstructions,
		Tags:                    raw.Tags,
		AlternateGreetings:      raw.AlternateGreetings,
		CharacterBook:           raw.CharacterBook,
	}
	if raw.Data != nil {
		card.Name = firstNonEmpty(raw.Data.Name, card.Name)
		card.Description = firstNonEmpty(raw.Data.Description, card.Description)
		card.Personality = firstNonEmpty(raw.Data.Personality, card.Personality)
		card.Scenario = firstNonEmpty(raw.Data.Scenario, card.Scenario)
		card.FirstMes = firstNonEmpty(raw.Data.FirstMes, card.FirstMes)
		card.MesExample = firstNonEmpty(raw.Data.MesExample, card.MesExample)
		card.CreatorNotes = firstNonEmpty(raw.Data.CreatorNotes, card.CreatorNotes)
		card.SystemPrompt = firstNonEmpty(raw.Data.SystemPrompt, card.SystemPrompt)
		card.PostHistoryInstructions = firstNonEmpty(raw.Data.PostHistoryInstructions, card.PostHistoryInstructions)
		if len(raw.Data.Tags) > 0 {
			card.Tags = raw.Data.Tags
		}
		if len(raw.Data.AlternateGreetings) > 0 {
			card.AlternateGreetings = raw.Data.AlternateGreetings
		}
		if raw.Data.CharacterBook != nil {
			card.CharacterBook = raw.Data.CharacterBook
		}
	}
	card.Name = strings.TrimSpace(card.Name)
	return card, nil
}

func renderTavernCardMarkdown(card normalizedTavernCard, source string, importedAt time.Time) string {
	var sb strings.Builder
	sb.WriteString("<!-- tavern-card-import:start -->\n")
	sb.WriteString("## 酒馆角色卡：")
	sb.WriteString(card.Name)
	sb.WriteString("\n\n")
	sb.WriteString("- 来源文件：")
	sb.WriteString(source)
	sb.WriteString("\n")
	sb.WriteString("- 导入时间：")
	sb.WriteString(importedAt.Format(time.RFC3339))
	sb.WriteString("\n")
	if card.Spec != "" || card.SpecVersion != "" {
		sb.WriteString("- 格式：")
		sb.WriteString(strings.TrimSpace(card.Spec + " " + card.SpecVersion))
		sb.WriteString("\n")
	}
	if len(card.Tags) > 0 {
		sb.WriteString("- 标签：")
		sb.WriteString(strings.Join(card.Tags, "、"))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	writeMarkdownSection(&sb, "角色描述", card.Description)
	writeMarkdownSection(&sb, "性格", card.Personality)
	writeMarkdownSection(&sb, "场景", card.Scenario)
	writeMarkdownSection(&sb, "开场白", card.FirstMes)
	writeMarkdownSection(&sb, "对话示例", card.MesExample)
	writeMarkdownSection(&sb, "作者备注", card.CreatorNotes)
	writeMarkdownSection(&sb, "系统提示", card.SystemPrompt)
	writeMarkdownSection(&sb, "历史后置提示", card.PostHistoryInstructions)

	if len(card.AlternateGreetings) > 0 {
		sb.WriteString("### 备用开场白\n\n")
		for i, greeting := range card.AlternateGreetings {
			if strings.TrimSpace(greeting) == "" {
				continue
			}
			sb.WriteString("#### 备用开场白 ")
			sb.WriteString(strconv.Itoa(i + 1))
			sb.WriteString("\n\n")
			sb.WriteString(normalizeCardText(greeting))
			sb.WriteString("\n\n")
		}
	}

	if card.CharacterBook != nil {
		writeCharacterBookMarkdown(&sb, card.CharacterBook)
	}
	sb.WriteString("<!-- tavern-card-import:end -->\n")
	return sb.String()
}

func writeMarkdownSection(sb *strings.Builder, title, content string) {
	content = normalizeCardText(content)
	if content == "" {
		return
	}
	sb.WriteString("### ")
	sb.WriteString(title)
	sb.WriteString("\n\n")
	sb.WriteString(content)
	sb.WriteString("\n\n")
}

func writeCharacterBookMarkdown(sb *strings.Builder, book *tavernCharacterBook) {
	if book == nil || len(book.Entries) == 0 {
		return
	}
	sb.WriteString("### 世界书 / 角色条目\n\n")
	if strings.TrimSpace(book.Name) != "" {
		sb.WriteString("- 世界书名称：")
		sb.WriteString(strings.TrimSpace(book.Name))
		sb.WriteString("\n\n")
	}
	for i, entry := range book.Entries {
		if entry.Enabled != nil && !*entry.Enabled && strings.TrimSpace(entry.Content) == "" {
			continue
		}
		title := firstNonEmpty(entry.Comment, strings.Join(entry.Keys, "、"), fmt.Sprintf("条目 %d", i+1))
		sb.WriteString("#### ")
		sb.WriteString(title)
		sb.WriteString("\n\n")
		if len(entry.Keys) > 0 {
			sb.WriteString("- 关键词：")
			sb.WriteString(strings.Join(entry.Keys, "、"))
			sb.WriteString("\n")
		}
		if len(entry.SecondaryKeys) > 0 {
			sb.WriteString("- 次级关键词：")
			sb.WriteString(strings.Join(entry.SecondaryKeys, "、"))
			sb.WriteString("\n")
		}
		sb.WriteString("- 启用：")
		if entry.Enabled == nil {
			sb.WriteString("未声明")
		} else if *entry.Enabled {
			sb.WriteString("是")
		} else {
			sb.WriteString("否")
		}
		sb.WriteString("\n\n")
		content := normalizeCardText(entry.Content)
		if content != "" {
			sb.WriteString(content)
			sb.WriteString("\n\n")
		}
	}
}

func (s *Service) appendCharactersMarkdown(content string) error {
	absPath, err := SafePath(s.workspace, charactersFilePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	existing, err := os.ReadFile(absPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var next strings.Builder
	if len(bytes.TrimSpace(existing)) == 0 {
		next.WriteString("# 角色卡片\n\n")
	} else {
		next.Write(existing)
		if !bytes.HasSuffix(existing, []byte("\n")) {
			next.WriteString("\n")
		}
		next.WriteString("\n")
	}
	next.WriteString(content)
	return os.WriteFile(absPath, []byte(next.String()), 0o644)
}

func normalizeCardText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(text)
}

func characterBookEntryCount(book *tavernCharacterBook) int {
	if book == nil {
		return 0
	}
	return len(book.Entries)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
