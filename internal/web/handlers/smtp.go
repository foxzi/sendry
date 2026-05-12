package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/foxzi/sendry/internal/web/middleware"
	"github.com/foxzi/sendry/internal/web/models"
	smtpclient "github.com/foxzi/sendry/internal/web/smtp"
)

func (h *Handlers) SMTPList(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	servers, err := h.userSMTP.ListByUser(userID)
	if err != nil {
		h.logger.Error("list smtp servers", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to load SMTP servers")
		return
	}
	data := map[string]any{
		"Title":   "My SMTP Servers",
		"Active":  "settings",
		"User":    h.getUserFromContext(r),
		"Servers": servers,
	}
	h.render(w, "smtp_list", data)
}

func (h *Handlers) SMTPNew(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Title":  "Add SMTP Server",
		"Active": "settings",
		"User":   h.getUserFromContext(r),
	}
	h.render(w, "smtp_form", data)
}

func (h *Handlers) SMTPCreate(w http.ResponseWriter, r *http.Request) {
	if h.cipher == nil {
		h.error(w, http.StatusServiceUnavailable, "Encryption not configured. Set auth.encryption_key in web.yaml.")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form")
		return
	}
	srv, derr := parseSMTPForm(r)
	if derr != "" {
		h.error(w, http.StatusBadRequest, derr)
		return
	}
	enc, err := h.cipher.Encrypt(r.FormValue("password"))
	if err != nil {
		h.logger.Error("encrypt password", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to encrypt password")
		return
	}
	srv.PasswordEnc = enc
	srv.UserID = middleware.GetUserID(r)

	if err := h.userSMTP.Create(srv); err != nil {
		h.logger.Error("create smtp", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to save SMTP server")
		return
	}
	user := h.getUserFromContext(r)
	email, _ := user["Email"].(string)
	h.settings.LogAction(r, srv.UserID, email, "create", "smtp_server", srv.ID,
		auditJSON(map[string]any{"name": srv.Name, "host": srv.Host}))
	http.Redirect(w, r, "/settings/smtp", http.StatusSeeOther)
}

func (h *Handlers) SMTPEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userID := middleware.GetUserID(r)
	srv, err := h.userSMTP.GetByID(id, userID)
	if err != nil || srv == nil {
		h.error(w, http.StatusNotFound, "SMTP server not found")
		return
	}
	data := map[string]any{
		"Title":  "Edit SMTP Server",
		"Active": "settings",
		"User":   h.getUserFromContext(r),
		"Server": srv,
		"IsEdit": true,
	}
	h.render(w, "smtp_form", data)
}

func (h *Handlers) SMTPUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form")
		return
	}
	id := r.PathValue("id")
	userID := middleware.GetUserID(r)
	existing, err := h.userSMTP.GetByID(id, userID)
	if err != nil || existing == nil {
		h.error(w, http.StatusNotFound, "SMTP server not found")
		return
	}

	srv, derr := parseSMTPForm(r)
	if derr != "" {
		h.error(w, http.StatusBadRequest, derr)
		return
	}
	srv.ID = id
	srv.UserID = userID

	if newPass := r.FormValue("password"); newPass != "" {
		if h.cipher == nil {
			h.error(w, http.StatusServiceUnavailable, "Encryption not configured")
			return
		}
		enc, err := h.cipher.Encrypt(newPass)
		if err != nil {
			h.logger.Error("encrypt password", "error", err)
			h.error(w, http.StatusInternalServerError, "Failed to encrypt password")
			return
		}
		srv.PasswordEnc = enc
	}

	if err := h.userSMTP.Update(srv); err != nil {
		h.logger.Error("update smtp", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to save SMTP server")
		return
	}
	user := h.getUserFromContext(r)
	email, _ := user["Email"].(string)
	h.settings.LogAction(r, userID, email, "update", "smtp_server", srv.ID,
		auditJSON(map[string]any{"name": srv.Name}))
	http.Redirect(w, r, "/settings/smtp", http.StatusSeeOther)
}

func (h *Handlers) SMTPDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userID := middleware.GetUserID(r)
	if err := h.userSMTP.Delete(id, userID); err != nil {
		h.logger.Error("delete smtp", "error", err)
		h.error(w, http.StatusInternalServerError, "Failed to delete SMTP server")
		return
	}
	user := h.getUserFromContext(r)
	email, _ := user["Email"].(string)
	h.settings.LogAction(r, userID, email, "delete", "smtp_server", id, "{}")
	http.Redirect(w, r, "/settings/smtp", http.StatusSeeOther)
}

func (h *Handlers) SMTPTestConnection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userID := middleware.GetUserID(r)
	srv, err := h.userSMTP.GetByID(id, userID)
	if err != nil || srv == nil {
		h.json(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not found"})
		return
	}
	if h.cipher == nil {
		h.json(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "encryption not configured"})
		return
	}
	plain, err := h.cipher.Decrypt(srv.PasswordEnc)
	if err != nil {
		h.logger.Error("decrypt smtp password", "smtp_id", id, "error", err)
		h.json(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "stored password could not be decrypted — re-save it"})
		return
	}
	to := srv.FromAddress
	err = smtpclient.Send(smtpclient.Server{
		Host:       srv.Host,
		Port:       srv.Port,
		Username:   srv.Username,
		Password:   plain,
		Encryption: smtpclient.Encryption(srv.Encryption),
	}, smtpclient.Message{
		From:     srv.FromAddress,
		FromName: srv.FromName,
		To:       []string{to},
		Subject:  "[sendry] SMTP connection test",
		HTML:     "<p>If you see this message, your SMTP server is configured correctly.</p>",
	})
	if err != nil {
		h.json(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	h.json(w, http.StatusOK, map[string]any{"ok": true, "to": to})
}

func parseSMTPForm(r *http.Request) (*models.UserSMTPServer, string) {
	name := strings.TrimSpace(r.FormValue("name"))
	host := strings.TrimSpace(r.FormValue("host"))
	portStr := strings.TrimSpace(r.FormValue("port"))
	username := strings.TrimSpace(r.FormValue("username"))
	enc := strings.TrimSpace(r.FormValue("encryption"))
	fromAddr := strings.TrimSpace(r.FormValue("from_address"))
	fromName := strings.TrimSpace(r.FormValue("from_name"))

	if name == "" || host == "" || portStr == "" || username == "" || fromAddr == "" {
		return nil, "Name, host, port, username and from address are required"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return nil, "Port must be a number between 1 and 65535"
	}
	if enc != "ssl" && enc != "starttls" && enc != "none" {
		enc = "ssl"
	}
	return &models.UserSMTPServer{
		Name:        name,
		Host:        host,
		Port:        port,
		Username:    username,
		Encryption:  enc,
		FromAddress: fromAddr,
		FromName:    fromName,
	}, ""
}
