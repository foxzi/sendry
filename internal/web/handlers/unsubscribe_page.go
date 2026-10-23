package handlers

import (
	"html/template"
	"net/http"

	"github.com/foxzi/sendry/internal/web/models"
)

type unsubState int

const (
	unsubAsk unsubState = iota
	unsubAlready
	unsubDone
	unsubResubscribed
	unsubInvalid
	unsubError
	unsubNotInList
)

// unsubView is what the public unsubscribe page shows. Unsubscribing is per
// address book (the list the job was sent to), so the page speaks of "this
// mailing list" of the sender. The internal list name is not shown: it is an
// operator label, not meant for recipients.
type unsubView struct {
	State  unsubState
	Sender string
	Email  string
}

func newUnsubView(state unsubState, src *models.TrackingSource) unsubView {
	sender := src.FromName
	if sender == "" {
		sender = src.FromEmail
	}
	return unsubView{State: state, Sender: sender, Email: src.Email}
}

func renderUnsubPage(w http.ResponseWriter, status int, v unsubView) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.WriteHeader(status)
	_ = unsubTmpl.Execute(w, map[string]any{
		"V":            v,
		"Ask":          v.State == unsubAsk,
		"Already":      v.State == unsubAlready,
		"Done":         v.State == unsubDone,
		"Resubscribed": v.State == unsubResubscribed,
		"Invalid":      v.State == unsubInvalid,
		"Error":        v.State == unsubError,
		"NotInList":    v.State == unsubNotInList,
	})
}

var unsubTmpl = template.Must(template.New("unsub").Parse(`<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex, nofollow">
<title>Отписка от рассылки</title>
<style>
*{box-sizing:border-box}
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:16px;background:#f4f5f7;color:#1f2933;font:16px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif}
.card{width:100%;max-width:440px;background:#fff;border-radius:10px;box-shadow:0 2px 12px rgba(0,0,0,.08);padding:32px 28px;text-align:center}
h1{margin:0 0 12px;font-size:22px}
p{margin:0 0 20px;color:#52606d;word-break:break-word}
button{appearance:none;border:0;border-radius:6px;padding:12px 28px;font-size:16px;cursor:pointer;background:#2563eb;color:#fff}
button.secondary{background:#e4e7eb;color:#1f2933}
button:hover{opacity:.9}
</style>
</head>
<body>
<main class="card">
{{if .Ask}}
<h1>Отписка от рассылки</h1>
<p>Адрес {{.V.Email}} будет отписан от этого списка рассылки{{with .V.Sender}} {{.}}{{end}}. Другие рассылки, на которые вы подписаны, продолжат приходить.</p>
<form method="post"><input type="hidden" name="action" value="unsubscribe"><button type="submit">Отписаться</button></form>
{{else if .Already}}
<h1>Вы уже отписаны</h1>
<p>Адрес {{.V.Email}} уже отписан от этого списка рассылки{{with .V.Sender}} {{.}}{{end}}.</p>
<form method="post"><input type="hidden" name="action" value="resubscribe"><button type="submit" class="secondary">Вернуть подписку</button></form>
{{else if .Done}}
<h1>Вы отписались</h1>
<p>Адрес {{.V.Email}} больше не будет получать письма из этого списка рассылки{{with .V.Sender}} {{.}}{{end}}.</p>
<form method="post"><input type="hidden" name="action" value="resubscribe"><button type="submit" class="secondary">Вернуть подписку</button></form>
{{else if .Resubscribed}}
<h1>Подписка восстановлена</h1>
<p>Адрес {{.V.Email}} снова будет получать письма из этого списка рассылки{{with .V.Sender}} {{.}}{{end}}.</p>
{{else if .NotInList}}
<h1>Адреса нет в списке</h1>
<p>Адрес {{.V.Email}} уже удалён из этого списка рассылки{{with .V.Sender}} {{.}}{{end}}, письма на него больше не придут.</p>
{{else if .Invalid}}
<h1>Ссылка недействительна</h1>
<p>Возможно, она устарела или была скопирована не полностью.</p>
{{else}}
<h1>Что-то пошло не так</h1>
<p>Не удалось выполнить действие. Попробуйте ещё раз позже.</p>
{{end}}
</main>
</body>
</html>
`))
