package router

import "testing"

func TestRewriteAssetURLs(t *testing.T) {
	tests := []struct {
		name            string
		html            string
		publicURL       string
		publicUploadURL string
		want            string
	}{
		{
			name:            "no URLs configured leaves HTML untouched",
			html:            `<img src="/uploads/a.png"><img src="/static/b.png">`,
			publicURL:       "",
			publicUploadURL: "",
			want:            `<img src="/uploads/a.png"><img src="/static/b.png">`,
		},
		{
			name:            "only publicURL rewrites both uploads and static",
			html:            `<img src="/uploads/a.png"><img src="/static/b.png">`,
			publicURL:       "https://mail.example.com",
			publicUploadURL: "",
			want:            `<img src="https://mail.example.com/uploads/a.png"><img src="https://mail.example.com/static/b.png">`,
		},
		{
			name:            "publicUploadURL overrides uploads host, static uses publicURL",
			html:            `<img src="/uploads/a.png"><img src="/static/b.png">`,
			publicURL:       "https://mail.example.com",
			publicUploadURL: "https://cdn.example.com",
			want:            `<img src="https://cdn.example.com/uploads/a.png"><img src="https://mail.example.com/static/b.png">`,
		},
		{
			name:            "only publicUploadURL rewrites uploads, leaves static untouched",
			html:            `<img src="/uploads/a.png"><img src="/static/b.png">`,
			publicURL:       "",
			publicUploadURL: "https://cdn.example.com",
			want:            `<img src="https://cdn.example.com/uploads/a.png"><img src="/static/b.png">`,
		},
		{
			name:            "trailing slashes are trimmed",
			html:            `<img src="/uploads/a.png">`,
			publicURL:       "https://mail.example.com/",
			publicUploadURL: "https://cdn.example.com/",
			want:            `<img src="https://cdn.example.com/uploads/a.png">`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteAssetURLs(tc.html, tc.publicURL, tc.publicUploadURL)
			if got != tc.want {
				t.Errorf("rewriteAssetURLs() = %q, want %q", got, tc.want)
			}
		})
	}
}
