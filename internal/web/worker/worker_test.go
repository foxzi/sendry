package worker

import (
	"testing"
)

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		vars     map[string]string
		want     string
	}{
		{
			name:     "simple substitution",
			template: "Hello, {{name}}!",
			vars:     map[string]string{"name": "World"},
			want:     "Hello, World!",
		},
		{
			name:     "multiple variables",
			template: "{{greeting}}, {{name}}! Welcome to {{company}}.",
			vars: map[string]string{
				"greeting": "Hello",
				"name":     "John",
				"company":  "Acme Corp",
			},
			want: "Hello, John! Welcome to Acme Corp.",
		},
		{
			name:     "missing variable unchanged",
			template: "Hello, {{name}}! Your code is {{code}}.",
			vars:     map[string]string{"name": "John"},
			want:     "Hello, John! Your code is {{code}}.",
		},
		{
			name:     "no variables",
			template: "Hello, World!",
			vars:     map[string]string{"name": "John"},
			want:     "Hello, World!",
		},
		{
			name:     "empty template",
			template: "",
			vars:     map[string]string{"name": "John"},
			want:     "",
		},
		{
			name:     "variable with underscores",
			template: "Hello, {{first_name}}!",
			vars:     map[string]string{"first_name": "John"},
			want:     "Hello, John!",
		},
		{
			name:     "html content",
			template: "<h1>{{title}}</h1><p>{{content}}</p>",
			vars: map[string]string{
				"title":   "Welcome",
				"content": "This is a test.",
			},
			want: "<h1>Welcome</h1><p>This is a test.</p>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderTemplate(tt.template, tt.vars)
			if got != tt.want {
				t.Errorf("renderTemplate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMergeVariables(t *testing.T) {
	tests := []struct {
		name          string
		global        map[string]string
		campaign      map[string]string
		recipientJSON string
		wantKey       string
		wantValue     string
	}{
		{
			name:          "global only",
			global:        map[string]string{"company": "Global Corp"},
			campaign:      nil,
			recipientJSON: "",
			wantKey:       "company",
			wantValue:     "Global Corp",
		},
		{
			name:          "campaign overrides global",
			global:        map[string]string{"company": "Global Corp"},
			campaign:      map[string]string{"company": "Campaign Corp"},
			recipientJSON: "",
			wantKey:       "company",
			wantValue:     "Campaign Corp",
		},
		{
			name:          "recipient overrides all",
			global:        map[string]string{"name": "Global"},
			campaign:      map[string]string{"name": "Campaign"},
			recipientJSON: `{"name": "John"}`,
			wantKey:       "name",
			wantValue:     "John",
		},
		{
			name:          "all levels combined",
			global:        map[string]string{"global_var": "global"},
			campaign:      map[string]string{"campaign_var": "campaign"},
			recipientJSON: `{"recipient_var": "recipient"}`,
			wantKey:       "recipient_var",
			wantValue:     "recipient",
		},
		{
			name:          "invalid json ignored",
			global:        map[string]string{"name": "Global"},
			campaign:      nil,
			recipientJSON: "invalid json",
			wantKey:       "name",
			wantValue:     "Global",
		},
		{
			name:          "empty recipient json",
			global:        map[string]string{"name": "Global"},
			campaign:      map[string]string{"email": "test@example.com"},
			recipientJSON: "",
			wantKey:       "email",
			wantValue:     "test@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeVariables(tt.global, tt.campaign, tt.recipientJSON)
			if got[tt.wantKey] != tt.wantValue {
				t.Errorf("mergeVariables()[%s] = %q, want %q", tt.wantKey, got[tt.wantKey], tt.wantValue)
			}
		})
	}
}

func TestMergeVariables_AllPresent(t *testing.T) {
	global := map[string]string{
		"company":       "Global Corp",
		"support_email": "global@example.com",
	}
	campaign := map[string]string{
		"campaign_name": "Summer Sale",
		"support_email": "campaign@example.com",
	}
	recipientJSON := `{
		"name": "John Doe",
		"order_id": "12345"
	}`

	got := mergeVariables(global, campaign, recipientJSON)

	// Check all keys are present
	expectedKeys := []string{"company", "support_email", "campaign_name", "name", "order_id"}
	for _, key := range expectedKeys {
		if _, ok := got[key]; !ok {
			t.Errorf("mergeVariables() missing key %q", key)
		}
	}

	// Check priority: recipient > campaign > global
	if got["support_email"] != "campaign@example.com" {
		t.Errorf("mergeVariables()[support_email] = %q, want campaign override", got["support_email"])
	}
	if got["name"] != "John Doe" {
		t.Errorf("mergeVariables()[name] = %q, want recipient value", got["name"])
	}
}

func TestMapSendryStatus(t *testing.T) {
	tests := []struct {
		sendryStatus string
		want         string
	}{
		{"pending", "queued"},
		{"queued", "queued"},
		{"processing", "queued"},
		{"sent", "sent"},
		{"delivered", "sent"},
		{"failed", "failed"},
		{"bounced", "failed"},
		{"rejected", "failed"},
		{"unknown", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.sendryStatus, func(t *testing.T) {
			got := mapSendryStatus(tt.sendryStatus)
			if got != tt.want {
				t.Errorf("mapSendryStatus(%q) = %q, want %q", tt.sendryStatus, got, tt.want)
			}
		})
	}
}
