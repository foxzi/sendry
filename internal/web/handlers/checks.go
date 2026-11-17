package handlers

import (
	"fmt"
	"net/http"
)

// IPCheck handles IP DNSBL check page
func (h *Handlers) IPCheck(w http.ResponseWriter, r *http.Request) {
	serverName := r.PathValue("server")

	client, err := h.sendry.GetClient(serverName)
	if err != nil {
		h.error(w, r, http.StatusNotFound, "Server not found")
		return
	}

	ip := r.URL.Query().Get("ip")

	data := map[string]any{
		"Title":      fmt.Sprintf("%s - IP Check", serverName),
		"Active":     "servers",
		"User":       h.getUserFromContext(r),
		"ServerName": serverName,
		"IP":         ip,
	}

	// If IP is provided, perform the check
	if ip != "" {
		result, err := client.CheckIP(r.Context(), ip)
		if err != nil {
			h.logger.Error("failed to check IP", "error", err, "ip", ip)
			data["Error"] = err.Error()
		} else {
			data["Result"] = result
		}
	}

	h.render(w, "ip_check", data)
}
