# Рассылка: создание, правка и отправка одними строками

Заменяет пошаговый мастер (`campaign-wizard.ru.md`) и старые страницы правки.
Макет: `docs/campaign-edit-demo.html`, вкладки «Создание, версия 2» и «Правка, версия 2».

## Идея

Рассылка — это список строк. Строка в свёрнутом виде показывает сводку (что настроено),
в развёрнутом — поля. Одни и те же строки на трёх страницах:

| Страница | Режим (`Mode`) | Карточка «Письмо» | Карточка «Отправка» |
|---|---|---|---|
| `GET /campaigns/new` | `create` | шаги + необязательные строки | шаги, в конце «Отправить» / «Сохранить без отправки» |
| `GET /campaigns/{id}` | `edit` | каждая строка правится и сохраняется отдельно | нет |
| `GET /campaigns/{id}/send` | `send` | как в `edit` | шаги, в конце «Отправить N получателям» |

Строки «Письма»: `from` (От кого), `content` (Содержание), и в одной карточке-группе
необязательные `vars` (Переменные), `track` (Трекинг), `about` (Название и описание).
Строки «Отправки»: `to` (Кому), `check` (Проверка), `when` (Когда).

### Режим `create`
- Шаги по порядку: `from` → `content` → `to` → `check` → `when`. Открыт один шаг. «Далее» проверяет
  поля, сворачивает строку в сводку и открывает следующий шаг.
- Строки ниже текущего шага заблокированы (пунктир, «Изменить» не показан).
- `vars`, `track`, `about` заполнены по умолчанию, «Далее» их пропускает. Открываются после `content`.
  У группы подпись «Можно не менять».
- Пройденную строку можно открыть снова: кнопка «Готово», после неё снова открывается текущий шаг.
- «Сохранить без отправки» доступна после `content`; «Отправить N получателям» — после всех шагов.
- На сервер до финальной кнопки ничего не пишется: всё внутри одной `<form action="/campaigns">`.
- Название: если поле `name` пустое — тема версии A без `{{переменных}}` + `, ` + дата `ДД.ММ.ГГГГ`.
  JS показывает это название в заголовке страницы, сервер вычисляет его сам при сохранении.

### Режим `edit`
- Все строки свёрнуты. «Изменить» открывает строку (одна открыта за раз), внизу «Отмена» и «Сохранить».
- «Сохранить» → `POST /campaigns/{id}/section/{row}` (fetch, поля строки), ответ JSON.
  Успех: строка сворачивается, сводка пересобирается из полей, у названия строки на 2 с «Сохранено».
  Ошибка: текст ошибки в строке, строка остаётся открытой.
- «Отмена» возвращает поля к значениям на момент открытия.
- Сверху карточки «Письмо» подсказка: «Изменения коснутся следующих отправок. Уже отправленные письма не меняются.»

### Режим `send`
- «Письмо» как в `edit`. «Отправка» — шаги `to` → `check` → `when`, как в `create`, `to` открыт сразу.
- `<form action="/campaigns/{id}/send">` охватывает только карточку «Отправка».
- Если строка «Письма» открыта и не сохранена, отправка не проходит: «Сохраните или отмените изменения в строке «…»».
- Если у рассылки нет версий (нет шаблона), строка `content` помечена предупреждением, отправка недоступна.

## Сервер

### Данные страницы (все три режима)

| Ключ | Тип | Что |
|---|---|---|
| `Title`, `Active`, `User` | как везде | `Active` = `campaigns` |
| `Mode` | string | `create` / `edit` / `send` |
| `Campaign` | `*models.Campaign` | никогда не nil; для `create` — пустая, `TrackOpens`/`TrackClicks` = есть проксирующий домен |
| `Content` | `CampaignContent` | см. ниже; для `create` нулевая, `SplitA` = 50 |
| `Variables` | `[]VarPair` | `{Key, Value string}`, из `Campaign.Variables`, по ключу |
| `Templates` | `[]WizardTemplate` | `ID, Name, Folder, Subject, HasUnsubscribe` |
| `Folders` | `[]string` | папки шаблонов |
| `RecipientLists` | `[]models.RecipientList` | все списки |
| `Servers` | `[]config.SendryServer` | |
| `SenderDomains` | `map[string]WizardDomain` | ключ — домен в нижнем регистре |
| `UserEmail` | string | для пробного письма |
| `NoPublicURL` | bool | нет проксирующего домена |

Только на `GET /campaigns/{id}` дополнительно `Tracking` и `Jobs` (как сейчас).

`CampaignContent`: `ATemplateID, ASubject, BTemplateID, BSubject string; HasB bool; SplitA int; Extra int`.
Строится из *активных* версий (weight > 0) в порядке `GetVariants`: первая — A, вторая — B.
`ASubject`/`BSubject` — `SubjectOverride`, а если он пуст — тема шаблона.
`SplitA` = round(100·wA / (wA+wB)), ограничено 1–99; без B — 100. `Extra` — сколько активных версий сверх двух.

Помощник: `h.campaignFlowData(r, mode, c) (map[string]any, error)`.

### Маршруты

| Маршрут | Что |
|---|---|
| `GET /campaigns/new` | view `campaign_new` (`Mode=create`) |
| `POST /campaigns` | создание (контракт мастера без `tags`; `name` необязателен) |
| `GET /campaigns/{id}` | view `campaign_view` (`Mode=edit`) |
| `POST /campaigns/{id}/section/{row}` | сохранить строку `from`/`content`/`vars`/`track`/`about` |
| `GET /campaigns/{id}/send` | view `campaign_send` (`Mode=send`) |
| `POST /campaigns/{id}/send` | как сейчас; берёт только активные версии |
| `POST /campaigns/wizard/preview`, `/campaigns/wizard/test`, `GET /templates/{id}/frame` | без изменений |
| `GET /campaigns/{id}/edit`, `/variables`, `/variants` | 301 на `/campaigns/{id}` |

Удаляются: `PUT /campaigns/{id}`, `PUT /campaigns/{id}/variables`, `POST /campaigns/{id}/variants`,
`DELETE /campaigns/{id}/variants/{variantId}` и их обработчики; views `campaign_edit`, `campaign_variables`,
`campaign_variants`. `campaign_wizard.html` переименовывается в `campaign_new.html`.

### `POST /campaigns/{id}/section/{row}`

Тело — `application/x-www-form-urlencoded`, только поля строки. Ответ: `200 {"ok":true}` или
`{"error":"..."}` с 400 (проверка), 404 (нет рассылки или неизвестная строка), 500.
Пишет в журнал действий: `update`, `campaign`, id, `{"section":"<row>"}`.

| Строка | Поля | Проверка и запись |
|---|---|---|
| `from` | `from_email`, `from_name`, `reply_to` | `from_email` обязателен; оба адреса с `@` |
| `content` | `template_id`, `subject`, `ab_enabled`, `template_id_b`, `subject_b`, `split_a` | как `wizardVariants`; запись версий — ниже |
| `vars` | `variables` | пусто или JSON-объект строк |
| `track` | `track_opens`, `track_clicks`, `add_utm` | без домена открытия и клики принудительно выключены |
| `about` | `name`, `description` | `name` обязателен (после trim) |

Запись версий (`content`): для A и B по отдельности, с текущей активной версией на этом месте:
- была и шаблон с темой не поменялись — меняется только вес (`UpdateVariant`);
- была, но поменялись шаблон или тема — если версия не использовалась в отправках, обновляется на месте;
  если использовалась (DeleteVariant → `ErrInUse`), старая получает вес 0, добавляется новая с именем `A`/`B`;
- не было — добавляется (`A`/`B`);
- B убрали — удаляется, а если использовалась — вес 0.
Активные версии сверх двух (`Extra`) получают вес 0.

Новое в репозитории: `UpdateVariant(v)` — `name, template_id, subject_override, weight` по id.

### `POST /campaigns`

Поле `tags` больше не читается (колонка в базе остаётся). Пустой `name` → `autoCampaignName(subject, now)`:
тема версии A (или тема шаблона A, если тема пуста) без `{{…}}`, обрезанная по краям от пробелов и `,.!:;-`,
сжатые пробелы, + `, ДД.ММ.ГГГГ`; если от темы ничего не осталось — `Campaign ДД.ММ.ГГГГ`.

## Разметка (контракт views ↔ JS ↔ CSS)

Общие куски — в `views/_campaign_flow.html` (`views.go` подключает файлы с `_` в начале к layout как
общие шаблоны и не делает из них страниц): `{{define "cf_letter"}}` и `{{define "cf_send"}}`.
Скрипты: `/static/js/i18n/campaign-flow.js` подключается в `layout.html` после `i18n/campaigns.js`;
`/static/js/campaign-flow.js` — в конце content каждой из трёх страниц, перед ним
`window.CF_CONFIG = {mode, campaignId, noPublicURL, isAdmin, userEmail, senderDomains}`.

```
div.cf#cf [data-mode=create|edit|send] [data-campaign-id]
  (create) form#cf-form method=post action=/campaigns novalidate   — обёртка обеих карточек
  h2.cf-title                       «Письмо» / «Отправка»
  p.cf-note                         (edit, send) подсказка про следующие отправки
  div.cf-rows                       ряд карточек
    section.cf-row [data-row] [data-kind=step|opt]
      div.cf-sum
        span.cf-dot (aria-hidden)
        h3.cf-name#cf-{row}-h
        p.cf-hint                   одна строка «что здесь настраивается», видна только в открытой строке
        div.cf-val [data-out]       сводка, заполняет JS
        button.cf-edit-btn [data-act=open] aria-expanded aria-controls=cf-{row}-body   «Изменить»
      div.cf-body#cf-{row}-body [hidden]
        div.cf-fields               поля
        p.cf-error role=alert hidden
        div.cf-foot
          button.btn [data-act=cancel]           «Отмена» (видна в edit/send-«Письмо»)
          button.btn.btn-primary [data-act=primary]  текст ставит JS: Далее / Готово / Сохранить
    div.cf-group                    карточка необязательных строк
      p.cf-group-h                  (create) «Можно не менять»
      section.cf-row …
  (send) form#cf-form method=post action=/campaigns/{id}/send novalidate — только вокруг «Отправки»
  div.cf-final                      (create, send)
    (create) button#cf-save type=submit name=action value=save  «Сохранить без отправки»
    button#cf-send type=submit name=action value=send           текст ставит JS
    p.cf-final-help#cf-final-help
```

Ошибки у полей: `small.cf-err[data-err=<имя поля>][role=alert][hidden]` сразу после поля.

Состояния строки (ставит JS): `.is-open`, `.is-locked`, `.is-done`; у точки `.cf-dot` классы
`ok` (✓), `cur` (номер, синяя), `todo` (номер, серая), `opt` (пустой кружок), `warn` (!).
Номера шагов JS проставляет сам; в `send` шаги «Отправки» нумеруются с 1.

Поля по строкам (имена — как у `POST /campaigns` и `/section`):

- `from`: `input#from_name[name=from_name]`, `input#from_email[name=from_email][type=email]`,
  `input#reply_to[name=reply_to][type=email]` — подпись «Reply-To», пояснение «Куда придут ответы получателей».
- `content`:
  - `input[type=hidden]#template_id[name=template_id]`, `#template_id_b[name=template_id_b]`,
    `input[type=hidden]#ab_enabled[name=ab_enabled][value=on]` (JS ставит `disabled`, когда B нет);
  - `div.cf-vtabs` (hidden без B) с `span.cf-vtab` на версию: `button[data-ver=a|b]` (у текущей `.on`) и крестик
    `button.cf-vtab-x[data-act=ver-del][data-del=a|b]`. Крестик у B убирает B; крестик у A убирает A, и B становится A;
    `p.cf-ab-help` (hidden без B) — «Каждый получатель получит одну из версий, случайно…»;
  - `div.cf-gal-tools`: `input[type=search]#cf-tpl-search.cf-search`, `div.cf-chips` с
    `button.cf-chip[data-folder]` («Все» = `data-folder=""`, у выбранной `.on`);
  - `div.cf-gal#cf-gal` с `button.cf-tpl[data-id][data-name][data-folder][data-subject][data-unsub=1|0][aria-pressed]`
    → `span.cf-thumb > iframe[src=/templates/{id}/frame][loading=lazy][sandbox=""][tabindex=-1][aria-hidden=true][title=""]`,
    `span.cf-tpl-name`, `span.cf-tpl-folder`; выбранная — `.sel`. `p.cf-empty#cf-gal-empty[hidden]` «Ничего не нашлось».
    Без шаблонов — `alert-warning` со ссылкой «Создать шаблон».
  - `div.field[data-ver-field=a]` с `label[for=subject]#cf-subject-label` + `input#subject[name=subject]`;
    `div.field[data-ver-field=b][hidden]` с `label[for=subject_b]#cf-subject-b-label` + `input#subject_b[name=subject_b]`;
  - `div.cf-split[hidden]`: `label[for=split_a]`, `input[type=range]#split_a[name=split_a][min=1][max=99]`, `span.cf-split-label#cf-split-label`;
  - `button.cf-link[data-act=ab]` «+ Добавить версию B (A/B-тест)», скрыта, когда B есть;
  - (edit/send, `Content.Extra > 0`) `p.cf-note` «У рассылки ещё N версий. При сохранении они отключатся.»
  - Сводка (JS): одна версия — `div.cf-ver` (мини-миниатюра `span.cf-mini > iframe` того же frame, название шаблона,
    `span.cf-sub` «тема»); две — `div.cf-vers` с двумя `div.cf-ver`, у каждой `b` «Версия A», `span.cf-pct` «50% получателей».
- `vars`: `input[type=hidden]#variables[name=variables]`; `div.cf-kv#cf-kv` с `div.cf-kv-row` →
  `input.input.cf-kv-key`, `input.input.cf-kv-val`, `button.cf-kv-del[data-act=kv-del][aria-label]`;
  `button.cf-link[data-act=kv-add]` «+ Добавить переменную»; `button.cf-link[data-act=kv-json]` «Править как JSON»;
  `textarea.input.code#cf-vars-json[hidden]`. Сервер выводит `Variables` строками, без них — одна пустая строка.
  Сводка: `span.cf-kvs > code` «ключ = значение» или «Нет».
- `track`: `track_opens`, `track_clicks` (без домена — сняты и `disabled`), `add_utm` — `label.checkbox-label`;
  подсказки `trk.*` как сейчас.
- `about`: `input#name[name=name]` (create: placeholder и пояснение «Если пусто — тема версии A и дата»),
  `textarea#description[name=description]`.
- `to`: `div.cf-lists#cf-lists[role=radiogroup]` с `label.cf-list[data-name][data-active][data-total]` →
  `input[type=radio][name=recipient_list_id][value]`, `span.cf-list-name`, `span.cf-list-num` (`b` число + `small[data-role=inactive]`).
  Без списков — `alert-warning` со ссылкой «Создать список».
- `check`: `div.cf-check` → `div.cf-panel` (превью): `div.cf-seg#cf-dev` (`button[data-dev=pc|phone]`),
  `div.cf-seg#cf-pv-ver[hidden]` (`button[data-ver=a|b]`), `p.cf-pv-subject` → `span#cf-pv-subject`,
  `div.cf-stage#cf-stage` → `iframe.cf-preview#cf-preview[sandbox=""]` + `p.cf-stage-msg#cf-preview-msg[hidden]`;
  `div.cf-panel` (пробное письмо): `input#cf-test-to[type=email][value=UserEmail]`, `button.btn#cf-test-btn`,
  `p.cf-test-result#cf-test-result[role=status][hidden]`; `div.cf-panel` (готовность): `div.cf-ready#cf-ready`.
- `when`: `div.cf-radio-row` с `input[type=radio][name=cf_when][value=now|later]`;
  `div#cf-when-later[hidden]`: `input#scheduled_at[name=scheduled_at][type=datetime-local][disabled]`,
  `input[type=hidden]#tz[name=tz]`, `#tz_offset[name=tz_offset]`, `span#cf-tz-label`;
  `input[type=checkbox]#dry_run[name=dry_run][value=on]`, `div.cf-pilot#cf-pilot[hidden]` с
  `input#dry_run_limit[name=dry_run_limit][type=number][min=1][max=100][value=10][disabled]`;
  `details.cf-more` с `input[type=checkbox][name=servers][value][checked]` по серверам и `select#strategy[name=strategy]`.

Превью и пробное письмо — как в мастере (`/campaigns/wizard/preview`, `/campaigns/wizard/test`),
ширина 600 / 375, переключатель A/B, готовность: ссылка отписки во всех выбранных шаблонах,
проксирующий домен, домен отправителя (известен / развёрнут / DKIM).

## Переводы

- Статичный текст разметки — ключи `cmp.cf_*` в `static/js/i18n/campaigns.js` (en и ru).
- Текст, который собирает JS, — ключи `cf.*` в новом `static/js/i18n/campaign-flow.js` (en и ru);
  JS может брать и существующие `cmp.wz_*`, `trk.*`.
- Старые ключи пока не удаляются; неиспользуемые вычищаются в конце.

## Порядок работ

Параллельно: сервер (handlers, repository, server.go, тесты), разметка (views, views.go, layout, `cmp.cf_*`),
скрипт (`campaign-flow.js` вместо `campaign-wizard.js`, `i18n/campaign-flow.js`), стили (`cf-*` вместо `wz-*`).
Потом сборка, тесты, проверка в панели (`make docker-up`, localhost:8088).
