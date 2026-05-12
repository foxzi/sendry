package worker

import (
	"testing"
)

func TestRenderTemplate_Flat(t *testing.T) {
	cases := []struct {
		name     string
		template string
		vars     map[string]any
		want     string
	}{
		{
			name:     "simple substitution",
			template: "Hello, {{name}}!",
			vars:     map[string]any{"name": "World"},
			want:     "Hello, World!",
		},
		{
			name:     "multiple variables",
			template: "{{greeting}}, {{name}}! Welcome to {{company}}.",
			vars: map[string]any{
				"greeting": "Hello",
				"name":     "John",
				"company":  "Acme Corp",
			},
			want: "Hello, John! Welcome to Acme Corp.",
		},
		{

			name:     "missing variable empty",
			template: "Hello, {{name}}! Your code is {{code}}.",
			vars:     map[string]any{"name": "John"},
			want:     "Hello, John! Your code is .",
		},
		{
			name:     "no variables",
			template: "Hello, World!",
			vars:     map[string]any{"name": "John"},
			want:     "Hello, World!",
		},
		{
			name:     "empty template",
			template: "",
			vars:     map[string]any{"name": "John"},
			want:     "",
		},
		{
			name:     "variable with underscores",
			template: "Hello, {{first_name}}!",
			vars:     map[string]any{"first_name": "John"},
			want:     "Hello, John!",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderTemplate("t", tt.template, tt.vars)
			if err != nil {
				t.Fatalf("renderTemplate err: %v", err)
			}
			if got != tt.want {
				t.Errorf("renderTemplate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderTemplate_RangeIf(t *testing.T) {
	tmpl := `{{if Items}}<ul>{{range Items}}<li>{{.Name}}</li>{{end}}</ul>{{end}}`
	out, err := renderTemplate("t", tmpl, map[string]any{
		"Items": []map[string]any{
			{"Name": "A"},
			{"Name": "B"},
		},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "<ul><li>A</li><li>B</li></ul>"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}

	out, _ = renderTemplate("t", tmpl, map[string]any{"Items": []any{}})
	if out != "" {
		t.Errorf("expected empty, got %q", out)
	}
}

func TestMergeVariables(t *testing.T) {
	cases := []struct {
		name          string
		global        map[string]string
		campaign      map[string]string
		recipientJSON string
		wantKey       string
		want          any
	}{
		{
			name:          "global only",
			global:        map[string]string{"company": "Global Corp"},
			campaign:      nil,
			recipientJSON: "",
			wantKey:       "company",
			want:          "Global Corp",
		},
		{
			name:          "campaign overrides global",
			global:        map[string]string{"company": "Global Corp"},
			campaign:      map[string]string{"company": "Campaign Corp"},
			recipientJSON: "",
			wantKey:       "company",
			want:          "Campaign Corp",
		},
		{
			name:          "recipient overrides all (string)",
			global:        map[string]string{"name": "Global"},
			campaign:      map[string]string{"name": "Campaign"},
			recipientJSON: `{"name": "John"}`,
			wantKey:       "name",
			want:          "John",
		},
		{
			name:          "recipient with array",
			global:        nil,
			campaign:      nil,
			recipientJSON: `{"Items": [{"Name": "A"}, {"Name": "B"}]}`,
			wantKey:       "Items",
			want: nil,
		},
		{
			name:          "invalid json ignored",
			global:        map[string]string{"name": "Global"},
			campaign:      nil,
			recipientJSON: "invalid json",
			wantKey:       "name",
			want:          "Global",
		},
		{
			name:          "empty recipient json",
			global:        map[string]string{"name": "Global"},
			campaign:      map[string]string{"email": "test@example.com"},
			recipientJSON: "",
			wantKey:       "email",
			want:          "test@example.com",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeVariables(tt.global, tt.campaign, tt.recipientJSON)
			val, ok := got[tt.wantKey]
			if !ok {
				t.Fatalf("missing key %q", tt.wantKey)
			}
			if tt.want != nil && val != tt.want {
				t.Errorf("mergeVariables()[%s] = %v, want %v", tt.wantKey, val, tt.want)
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

	expectedKeys := []string{"company", "support_email", "campaign_name", "name", "order_id"}
	for _, key := range expectedKeys {
		if _, ok := got[key]; !ok {
			t.Errorf("mergeVariables() missing key %q", key)
		}
	}

	if got["support_email"] != "campaign@example.com" {
		t.Errorf("mergeVariables()[support_email] = %v, want campaign override", got["support_email"])
	}
	if got["name"] != "John Doe" {
		t.Errorf("mergeVariables()[name] = %v, want recipient value", got["name"])
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
