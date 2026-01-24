package handlers

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// LoginPage renders the login page
func (h *Handlers) LoginPage(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"LocalEnabled":  h.cfg.Auth.LocalEnabled,
		"OIDCEnabled":   h.cfg.Auth.OIDC.Enabled,
		"OIDCProvider":  h.cfg.Auth.OIDC.Provider,
	}
	h.render(w, "login", data)
}

// Login handles login form submission
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderLoginError(w, "Invalid form data")
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	// Get user from DB
	var userID, passwordHash string
	err := h.db.QueryRow(
		"SELECT id, password_hash FROM users WHERE email = ?",
		email,
	).Scan(&userID, &passwordHash)

	if err != nil {
		h.renderLoginError(w, "Invalid email or password")
		return
	}

	// Check password
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		h.renderLoginError(w, "Invalid email or password")
		return
	}

	// Create session
	h.createSession(w, userID, email)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Logout handles user logout
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	// Get session cookie
	cookie, err := r.Cookie("session")
	if err == nil {
		// Delete session from DB
		h.db.Exec("DELETE FROM sessions WHERE id = ?", cookie.Value)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
	})

	http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
}

// OIDCLogin initiates OIDC login flow
func (h *Handlers) OIDCLogin(w http.ResponseWriter, r *http.Request) {
	if h.oidc == nil {
		h.renderLoginError(w, "OIDC is not configured")
		return
	}

	url, state, err := h.oidc.AuthCodeURL()
	if err != nil {
		h.logger.Error("failed to generate auth URL", "error", err)
		h.renderLoginError(w, "Failed to initiate login")
		return
	}

	// Store state in cookie for validation
	http.SetCookie(w, &http.Cookie{
		Name:     "oidc_state",
		Value:    state,
		Path:     "/",
		MaxAge:   300, // 5 minutes
		HttpOnly: true,
		Secure:   h.cfg.Server.TLS.Enabled,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// OIDCCallback handles OIDC callback
func (h *Handlers) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	if h.oidc == nil {
		h.renderLoginError(w, "OIDC is not configured")
		return
	}

	// Get state from cookie
	stateCookie, err := r.Cookie("oidc_state")
	if err != nil {
		h.renderLoginError(w, "Invalid state")
		return
	}

	// Clear state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oidc_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	state := r.URL.Query().Get("state")
	if state != stateCookie.Value {
		h.renderLoginError(w, "Invalid state")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		errorDesc := r.URL.Query().Get("error_description")
		if errorDesc == "" {
			errorDesc = r.URL.Query().Get("error")
		}
		if errorDesc == "" {
			errorDesc = "Authorization failed"
		}
		h.renderLoginError(w, errorDesc)
		return
	}

	// Exchange code for user info
	userInfo, err := h.oidc.Exchange(r.Context(), state, code)
	if err != nil {
		h.logger.Error("OIDC exchange failed", "error", err)
		h.renderLoginError(w, "Authentication failed: "+err.Error())
		return
	}

	// Find or create user
	var userID string
	err = h.db.QueryRow("SELECT id FROM users WHERE email = ?", userInfo.Email).Scan(&userID)
	if err != nil {
		// Create new user from OIDC
		userID = uuid.New().String()
		_, err = h.db.Exec(
			"INSERT INTO users (id, email, name, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))",
			userID, userInfo.Email, userInfo.Name,
		)
		if err != nil {
			h.logger.Error("failed to create OIDC user", "error", err)
			h.renderLoginError(w, "Failed to create user")
			return
		}
		h.logger.Info("created OIDC user", "email", userInfo.Email)
	}

	// Create session
	h.createSession(w, userID, userInfo.Email)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handlers) createSession(w http.ResponseWriter, userID, email string) {
	sessionID := uuid.New().String()
	expiresAt := time.Now().Add(h.cfg.Auth.SessionTTL)

	_, err := h.db.Exec(
		"INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)",
		sessionID, userID, expiresAt,
	)
	if err != nil {
		h.logger.Error("failed to create session", "error", err, "email", email)
		return
	}

	// Set cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionID,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   h.cfg.Server.TLS.Enabled,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handlers) renderLoginError(w http.ResponseWriter, message string) {
	data := map[string]any{
		"LocalEnabled":  h.cfg.Auth.LocalEnabled,
		"OIDCEnabled":   h.cfg.Auth.OIDC.Enabled,
		"OIDCProvider":  h.cfg.Auth.OIDC.Provider,
		"Error":         message,
	}
	h.render(w, "login", data)
}
