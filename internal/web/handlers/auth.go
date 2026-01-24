package handlers

import (
	"net/http"
)

// LoginPage renders the login page
func (h *Handlers) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "login", nil)
}

// Login handles login form submission
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	// TODO: implement login
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Logout handles user logout
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	// TODO: implement logout
	http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
}

// OIDCCallback handles OIDC callback
func (h *Handlers) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	// TODO: implement OIDC callback
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
