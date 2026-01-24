package template

import (
	"testing"
)

func TestEngine_Validate(t *testing.T) {
	engine := NewEngine()

	tests := []struct {
		name    string
		tmpl    *Template
		wantErr bool
	}{
		{
			name: "valid template",
			tmpl: &Template{
				Subject: "Hello {{.Name}}",
				Text:    "Welcome {{.Name}}!",
				HTML:    "<p>Welcome {{.Name}}!</p>",
			},
			wantErr: false,
		},
		{
			name: "invalid subject syntax",
			tmpl: &Template{
				Subject: "Hello {{.Name",
				Text:    "Welcome",
			},
			wantErr: true,
		},
		{
			name: "invalid text syntax",
			tmpl: &Template{
				Subject: "Hello",
				Text:    "Welcome {{.Name",
			},
			wantErr: true,
		},
		{
			name: "invalid html syntax",
			tmpl: &Template{
				Subject: "Hello",
				HTML:    "<p>Welcome {{.Name</p>",
			},
			wantErr: true,
		},
		{
			name: "empty template",
			tmpl: &Template{
				Subject: "",
				Text:    "",
				HTML:    "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.Validate(tt.tmpl)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEngine_Render(t *testing.T) {
	engine := NewEngine()

	tests := []struct {
		name        string
		tmpl        *Template
		data        map[string]interface{}
		wantSubject string
		wantText    string
		wantHTML    string
		wantErr     bool
	}{
		{
			name: "simple render",
			tmpl: &Template{
				Subject: "Hello {{.Name}}",
				Text:    "Welcome {{.Name}}!",
				HTML:    "<p>Welcome {{.Name}}!</p>",
			},
			data:        map[string]interface{}{"Name": "John"},
			wantSubject: "Hello John",
			wantText:    "Welcome John!",
			wantHTML:    "<p>Welcome John!</p>",
			wantErr:     false,
		},
		{
			name: "missing variable",
			tmpl: &Template{
				Subject: "Hello {{.Name}}",
				Text:    "Welcome!",
			},
			data:        map[string]interface{}{},
			wantSubject: "Hello <no value>",
			wantText:    "Welcome!",
			wantErr:     false,
		},
		{
			name: "html escaping",
			tmpl: &Template{
				Subject: "Test",
				HTML:    "<p>{{.Content}}</p>",
			},
			data:        map[string]interface{}{"Content": "<script>alert('xss')</script>"},
			wantSubject: "Test",
			wantHTML:    "<p>&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;</p>",
			wantErr:     false,
		},
		{
			name: "complex data",
			tmpl: &Template{
				Subject: "Order #{{.OrderID}}",
				Text:    "Total: {{.Amount}} {{.Currency}}",
			},
			data: map[string]interface{}{
				"OrderID":  12345,
				"Amount":   99.99,
				"Currency": "USD",
			},
			wantSubject: "Order #12345",
			wantText:    "Total: 99.99 USD",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Render(tt.tmpl, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			if result.Subject != tt.wantSubject {
				t.Errorf("Render() subject = %v, want %v", result.Subject, tt.wantSubject)
			}
			if tt.wantText != "" && result.Text != tt.wantText {
				t.Errorf("Render() text = %v, want %v", result.Text, tt.wantText)
			}
			if tt.wantHTML != "" && result.HTML != tt.wantHTML {
				t.Errorf("Render() html = %v, want %v", result.HTML, tt.wantHTML)
			}
		})
	}
}
