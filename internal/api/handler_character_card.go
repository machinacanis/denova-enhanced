package api

import (
	"context"
	"io"
	"log"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

const maxCharacterCardUploadBytes int64 = 32 * 1024 * 1024

// handleWorkspaceImportCharacterCard POST /api/workspace/import-character-card — 导入酒馆角色卡 PNG/JSON。
func (s *Server) handleWorkspaceImportCharacterCard(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		writeError(c, consts.StatusBadRequest, "请上传 PNG 或 JSON 格式的酒馆角色卡文件")
		return
	}
	if fileHeader.Size > maxCharacterCardUploadBytes {
		writeError(c, consts.StatusBadRequest, "角色卡文件不能超过 32MB")
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		writeError(c, consts.StatusBadRequest, "读取上传文件失败: "+err.Error())
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxCharacterCardUploadBytes+1))
	if err != nil {
		writeError(c, consts.StatusBadRequest, "读取上传文件失败: "+err.Error())
		return
	}
	if int64(len(data)) > maxCharacterCardUploadBytes {
		writeError(c, consts.StatusBadRequest, "角色卡文件不能超过 32MB")
		return
	}

	log.Printf("[api] 导入酒馆角色卡 filename=%q size=%d workspace=%q", fileHeader.Filename, len(data), s.app.Workspace())
	result, err := s.app.BookService().ImportTavernCharacterCard(fileHeader.Filename, data)
	if err != nil {
		log.Printf("[api] 导入酒馆角色卡失败 filename=%q error=%v", fileHeader.Filename, err)
		writeError(c, consts.StatusBadRequest, "导入酒馆角色卡失败: "+err.Error())
		return
	}
	log.Printf("[api] 导入酒馆角色卡完成 name=%q target=%q entries=%d", result.Name, result.TargetPath, result.EntryCount)
	writeJSON(c, consts.StatusOK, result)
}
