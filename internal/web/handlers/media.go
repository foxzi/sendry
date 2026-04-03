package handlers

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/foxzi/sendry/internal/web/middleware"
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/google/uuid"
)

var allowedMimeTypes = map[string]bool{
	"image/png":     true,
	"image/jpeg":    true,
	"image/gif":     true,
	"image/webp":    true,
	"image/svg+xml": true,
}

var allowedExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".svg":  true,
}

func (h *Handlers) MediaList(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	pageStr := r.URL.Query().Get("page")

	page := 1
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}
	limit := 24
	offset := (page - 1) * limit

	files, total, err := h.media.List(models.MediaListFilter{
		Search: search,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		h.logger.Error("failed to list media", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to list media")
		return
	}

	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	data := map[string]any{
		"Title":      "Media",
		"Active":     "media",
		"User":       h.getUserFromContext(r),
		"Files":      files,
		"Search":     search,
		"Page":       page,
		"TotalPages": totalPages,
		"Total":      total,
	}

	h.render(w, "media", data)
}

func (h *Handlers) MediaUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		h.error(w, http.StatusBadRequest, "File too large (max 10MB)")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.error(w, http.StatusBadRequest, "No file uploaded")
		return
	}
	defer file.Close()

	mimeType := header.Header.Get("Content-Type")
	if !allowedMimeTypes[mimeType] {
		h.error(w, http.StatusBadRequest, "Only images are allowed (PNG, JPEG, GIF, WebP, SVG)")
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExtensions[ext] {
		h.error(w, http.StatusBadRequest, "File extension not allowed. Accepted: .png, .jpg, .jpeg, .gif, .webp, .svg")
		return
	}

	sniffBuf := make([]byte, 512)
	n, _ := file.Read(sniffBuf)
	sniffBuf = sniffBuf[:n]
	detectedType := http.DetectContentType(sniffBuf)
	if seeker, ok := file.(io.Seeker); ok {
		seeker.Seek(0, io.SeekStart)
	}

	if detectedType == "text/xml; charset=utf-8" || detectedType == "text/plain; charset=utf-8" {
		if bytes.Contains(sniffBuf, []byte("<svg")) {
			detectedType = "image/svg+xml"
		}
	}

	if !allowedMimeTypes[detectedType] {
		h.error(w, http.StatusBadRequest, "File content does not match an allowed image type")
		return
	}

	uploadPath := h.cfg.Server.UploadPath
	if err := os.MkdirAll(uploadPath, 0755); err != nil {
		h.logger.Error("failed to create upload dir", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to save file")
		return
	}

	storedName := uuid.New().String() + ext

	dst, err := os.Create(filepath.Join(uploadPath, storedName))
	if err != nil {
		h.logger.Error("failed to create file", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to save file")
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		h.logger.Error("failed to write file", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to save file")
		return
	}

	url := "/uploads/" + storedName

	m := &models.MediaFile{
		Name:     storedName,
		OrigName: sanitizeFilename(header.Filename),
		MimeType: mimeType,
		Size:     written,
		URL:      url,
	}

	if err := h.media.Create(m); err != nil {
		h.logger.Error("failed to save media record", "error", err)
		os.Remove(filepath.Join(uploadPath, storedName))
		h.error(w, http.StatusInternalServerError, "Failed to save file")
		return
	}

	user := h.getUserFromContext(r)
	h.settings.LogAction(r, middleware.GetUserID(r), user["Email"].(string),
		"upload", "media", m.ID, auditJSON(map[string]any{"name": m.OrigName, "size": m.Size}))

	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		h.json(w, http.StatusOK, m)
		return
	}

	http.Redirect(w, r, "/media", http.StatusSeeOther)
}

func (h *Handlers) MediaDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	m, err := h.media.GetByID(id)
	if err != nil {
		h.logger.Error("failed to get media", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load media")
		return
	}
	if m == nil {
		h.error(w, http.StatusNotFound, "File not found")
		return
	}

	filePath := filepath.Join(h.cfg.Server.UploadPath, m.Name)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		h.logger.Error("failed to delete file from disk", "error", err, "path", filePath)
	}

	if err := h.media.Delete(id); err != nil {
		h.logger.Error("failed to delete media", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to delete file")
		return
	}

	user := h.getUserFromContext(r)
	h.settings.LogAction(r, middleware.GetUserID(r), user["Email"].(string),
		"delete", "media", id, auditJSON(map[string]any{"name": m.OrigName}))

	http.Redirect(w, r, "/media", http.StatusSeeOther)
}

func (h *Handlers) MediaListJSON(w http.ResponseWriter, r *http.Request) {
	files, _, err := h.media.List(models.MediaListFilter{Limit: 200})
	if err != nil {
		h.json(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load media"})
		return
	}
	if files == nil {
		files = []models.MediaFile{}
	}
	h.json(w, http.StatusOK, files)
}
