package handlers

import (
	"net/http"

	"github.com/foxzi/sendry/internal/web/middleware"
	"github.com/foxzi/sendry/internal/web/models"
)

// UserList shows all users
func (h *Handlers) UserList(w http.ResponseWriter, r *http.Request) {
	users, err := h.settings.ListUsers()
	if err != nil {
		h.error(w, http.StatusInternalServerError, "Failed to load users")
		return
	}

	data := map[string]any{
		"Title":  "Users",
		"Active": "settings",
		"User":   h.getUserFromContext(r),
		"Users":  users,
	}

	h.render(w, "settings_users", data)
}

// UserNew shows the create user form
func (h *Handlers) UserNew(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Title":  "New User",
		"Active": "settings",
		"User":   h.getUserFromContext(r),
	}
	h.render(w, "settings_user_form", data)
}

// UserCreate handles user creation
func (h *Handlers) UserCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	email := r.FormValue("email")
	name := r.FormValue("name")
	password := r.FormValue("password")
	role := models.UserRole(r.FormValue("role"))

	if email == "" || password == "" {
		data := map[string]any{
			"Title":  "New User",
			"Active": "settings",
			"User":   h.getUserFromContext(r),
			"Error":  "Email and password are required",
		}
		h.render(w, "settings_user_form", data)
		return
	}
	if role != models.RoleAdmin && role != models.RoleUser {
		role = models.RoleUser
	}

	created, err := h.settings.CreateUser(email, name, password, role)
	if err != nil {
		data := map[string]any{
			"Title":  "New User",
			"Active": "settings",
			"User":   h.getUserFromContext(r),
			"Error":  "Failed to create user: " + err.Error(),
		}
		h.render(w, "settings_user_form", data)
		return
	}

	actorID := middleware.GetUserID(r)
	actorEmail := middleware.GetUserEmail(r)
	h.settings.LogAction(r, actorID, actorEmail, "create", "user", created.ID,
		`{"email":"`+email+`","role":"`+string(role)+`"}`)

	http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
}

// UserEdit shows the edit user form
func (h *Handlers) UserEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	target, err := h.settings.GetUserByID(id)
	if err != nil || target == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	data := map[string]any{
		"Title":    "Edit User",
		"Active":   "settings",
		"User":     h.getUserFromContext(r),
		"EditUser": target,
	}
	h.render(w, "settings_user_form", data)
}

// UserUpdate handles user update (name + role)
func (h *Handlers) UserUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	id := r.PathValue("id")
	name := r.FormValue("name")
	role := models.UserRole(r.FormValue("role"))

	if role != models.RoleAdmin && role != models.RoleUser {
		role = models.RoleUser
	}

	// Prevent removing the last admin
	currentUser, _ := h.settings.GetUserByID(id)
	if currentUser != nil && currentUser.Role == models.RoleAdmin && role != models.RoleAdmin {
		count, _ := h.settings.CountAdmins()
		if count <= 1 {
			target, _ := h.settings.GetUserByID(id)
			data := map[string]any{
				"Title":    "Edit User",
				"Active":   "settings",
				"User":     h.getUserFromContext(r),
				"EditUser": target,
				"Error":    "Cannot remove the last admin",
			}
			h.render(w, "settings_user_form", data)
			return
		}
	}

	if err := h.settings.UpdateUser(id, name, role); err != nil {
		h.error(w, http.StatusInternalServerError, "Failed to update user")
		return
	}

	actorID := middleware.GetUserID(r)
	actorEmail := middleware.GetUserEmail(r)
	h.settings.LogAction(r, actorID, actorEmail, "update", "user", id,
		`{"name":"`+name+`","role":"`+string(role)+`"}`)

	http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
}

// UserChangePassword handles password change
func (h *Handlers) UserChangePassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.error(w, http.StatusBadRequest, "Invalid form data")
		return
	}

	id := r.PathValue("id")
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")

	if password == "" || password != confirm {
		target, _ := h.settings.GetUserByID(id)
		data := map[string]any{
			"Title":    "Edit User",
			"Active":   "settings",
			"User":     h.getUserFromContext(r),
			"EditUser": target,
			"Error":    "Passwords do not match or are empty",
		}
		h.render(w, "settings_user_form", data)
		return
	}

	if err := h.settings.ChangePassword(id, password); err != nil {
		h.error(w, http.StatusInternalServerError, "Failed to change password")
		return
	}

	actorID := middleware.GetUserID(r)
	actorEmail := middleware.GetUserEmail(r)
	h.settings.LogAction(r, actorID, actorEmail, "update", "user", id, `{"field":"password"}`)

	http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
}

// UserDelete handles user deletion
func (h *Handlers) UserDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Prevent self-deletion
	currentUserID := middleware.GetUserID(r)
	if id == currentUserID {
		http.Error(w, "Cannot delete yourself", http.StatusBadRequest)
		return
	}

	// Prevent deleting last admin
	target, _ := h.settings.GetUserByID(id)
	if target != nil && target.Role == models.RoleAdmin {
		count, _ := h.settings.CountAdmins()
		if count <= 1 {
			http.Error(w, "Cannot delete the last admin", http.StatusBadRequest)
			return
		}
	}

	targetEmail := ""
	if target != nil {
		targetEmail = target.Email
	}

	if err := h.settings.DeleteUser(id); err != nil {
		h.error(w, http.StatusInternalServerError, "Failed to delete user")
		return
	}

	actorID := middleware.GetUserID(r)
	actorEmail := middleware.GetUserEmail(r)
	h.settings.LogAction(r, actorID, actorEmail, "delete", "user", id,
		`{"email":"`+targetEmail+`"}`)

	http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
}
