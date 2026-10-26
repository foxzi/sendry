# Рассылка: создание, правка и запуск

Заменяет пошаговый мастер (`campaign-wizard.ru.md`) и старые страницы правки.
Макет: `docs/campaign-edit-demo.html`, вкладки «Создание, версия 2» и «Правка, версия 2».

## Идея

Рассылка — это список строк. Строка в свёрнутом виде показывает сводку (что настроено),
в развёрнутом — поля. Письмо и запуск — разные вещи, у каждой своя страница:

- письмо (строки `from`, `content`, необязательные `vars`, `track`, `about`) собирают при создании
  и правят на странице рассылки;
- запуск (кому, когда, проверка) — одна страница `GET /campaigns/{id}/launch`, одинаковая для нового
  запуска и для правки запланированного. Новый запуск начинается только оттуда.

| Страница | Режим (`Mode`) | Что на ней |
|---|---|---|
| `GET /campaigns/new` | `create` | строки письма: шаги `from` → `content` + необязательные строки; внизу «Сохранить» и «Сохранить и отправить…» |
| `GET /campaigns/{id}` | `edit` | строки письма, каждая правится и сохраняется отдельно; «Отправить…» — ссылка на страницу запуска |
| `GET /campaigns/{id}/launch` | `launch` | шаги «Отправки»: `to` → `when` → `check`, в «Проверке» кнопка «Отправить N получателям» |
| `GET /campaigns/{id}/launch?job={jobID}` | `launch` + `Job` | то же для запланированного запуска: поля заполнены, форма уходит в `POST /jobs/{jobID}/schedule` |

Строки письма: `from` (От кого), `content` (Содержание), и в одной карточке-группе
необязательные `vars` (Переменные), `track` (Трекинг), `about` (Название и описание).
Строки запуска: `to` (Кому), `when` (Когда), `check` (Проверка) — в этом порядке.

### Режим `create`
- Шаги по порядку: `from` → `content`. Открыт один шаг. «Далее» проверяет поля, сворачивает строку в сводку
  и открывает следующий шаг. Мастер собирает только письмо; кому и когда отправлять — на странице запуска.
- Строка ниже текущего шага заблокирована (пунктир, «Изменить» не показан).
- `vars`, `track`, `about` заполнены по умолчанию, «Далее» их пропускает. Открываются после `content`.
  У группы подпись «Можно не менять».
- Пройденную строку можно открыть снова: кнопка «Готово», после неё снова открывается текущий шаг.
- Внизу две кнопки, обе доступны после `content`: «Сохранить» (`action=save`, создаёт рассылку и ведёт на
  `/campaigns/{id}`) и «Сохранить и отправить…» (`action=launch`, создаёт и ведёт на `/campaigns/{id}/launch`).
  Задание на отправку мастер не создаёт.
- На сервер до кнопки ничего не пишется: всё внутри одной `<form action="/campaigns">`. Пока в форме что-то
  менялось, уход со страницы спрашивает подтверждение (`beforeunload`).
- Название: если поле `name` пустое — тема версии A без `{{переменных}}` + `, ` + дата `ДД.ММ.ГГГГ`.
  JS показывает это название в заголовке страницы, сервер вычисляет его сам при сохранении.

### Режим `edit`
- Все строки свёрнуты. «Изменить» открывает строку (одна открыта за раз), внизу «Отмена» и «Сохранить».
- «Сохранить» → `POST /campaigns/{id}/section/{row}` (fetch, поля строки), ответ JSON.
  Успех: строка сворачивается, сводка пересобирается из полей, у названия строки на 2 с «Сохранено».
  Ошибка: текст ошибки в строке, строка остаётся открытой.
- «Отмена» возвращает поля к значениям на момент открытия.
- Сверху карточки «Письмо» подсказка: «Изменения коснутся следующих отправок. Уже отправленные письма не меняются.»

### Режим `launch`: страница запуска
- Заголовок «Новый запуск: «<название>»» (в правке — «Изменить запуск»), справа ссылка «Отмена» на `/campaigns/{id}`.
- Шаги `to` → `when` → `check`, открыт один. В `check`: итог (список, сколько получат, когда, пилот),
  превью A/B, пробное письмо, готовность и одна кнопка «Отправить N получателям» / «Запланировать на …».
- Письмо на странице не правится: для превью, пробного письма и проверки домена отправителя оно лежит в скрытом
  блоке `{{define "cf_letter_data"}}` (поля `from_email`, `variables`, `template_id` … и по карточке `.cf-tpl`
  на использованный шаблон). Он стоит в `#cf` вне `<form>`, поэтому на сервер не уходит.
- `<form action="/campaigns/{id}/send">`; после успеха `303` на `/campaigns/{id}`, где запуск виден в списке.
- Если у рассылки нет версий (нет шаблона), кнопка отправки выключена с пояснением.
- Правка запланированного запуска (`?job={jobID}`): допустим только запуск этой рассылки со статусом `scheduled`,
  иначе 404. Список, серверы, стратегия и время заполнены из задания (время хранится в UTC: `#scheduled_at[data-utc]`,
  JS переводит его в часовой пояс браузера); страница открывается сразу на `check`, остальные шаги пройдены.
  Форма уходит в `POST /jobs/{jobID}/schedule` с теми же полями, что `POST /campaigns/{id}/send`
  (`recipient_list_id`, `servers[]`, `strategy`, `scheduled_at`, `tz`, `tz_offset`); пилота в этом режиме нет.
  «Отправить сейчас» вместо расписания остаётся доступным: тогда `scheduled_at` пуст.

## Сервер

### Данные страницы (оба режима)

| Ключ | Тип | Что |
|---|---|---|
| `Title`, `Active`, `User` | как везде | `Active` = `campaigns` |
| `Mode` | string | `create` / `edit` / `launch` |
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
Только на `GET /campaigns/{id}/launch` дополнительно `Job` (`*models.SendJob`, nil для нового запуска),
`JobServers` (`map[string]bool`, серверы задания) и `ScheduledUTC` (RFC 3339, пусто без расписания).

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
| `GET /campaigns/{id}/launch[?job={jobID}]` | view `campaign_launch` (`Mode=launch`); с `job` — правка запланированного запуска, чужой или не `scheduled` — 404 |
| `GET /campaigns/{id}/send` | 301 на `/campaigns/{id}/launch` |
| `POST /campaigns/{id}/send` | как раньше; берёт только активные версии; успех — 303 на `/campaigns/{id}` |
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

Только создаёт рассылку и версии. Поле `action`: `save` или пусто — 303 на `/campaigns/{id}`, `launch` — 303 на
`/campaigns/{id}/launch`, другое — 400. Полей отправки (`recipient_list_id`, `servers` …) форма не читает.
Поле `tags` больше не читается (колонка в базе остаётся). Пустой `name` → `autoCampaignName(subject, now)`:
тема версии A (или тема шаблона A, если тема пуста) без `{{…}}`, обрезанная по краям от пробелов и `,.!:;-`,
сжатые пробелы, + `, ДД.ММ.ГГГГ`; если от темы ничего не осталось — `Campaign ДД.ММ.ГГГГ`.

### `POST /jobs/{id}/schedule`: правка запланированного запуска

Принимает ту же форму, что `POST /campaigns/{id}/send` (`recipient_list_id`, `servers[]`, `strategy`, `scheduled_at`, `tz`, `tz_offset`); проверка — `parseJobParams`. Только для статуса `scheduled`, иначе 400; неизвестный запуск — 404.

- Список, серверы, стратегия и время обновляются, элементы строятся заново (`buildJobItems`, общий с созданием) по текущему составу списка. Настройки пробного запуска (dry-run) остаются как были. Список без активных получателей — 400, запуск не меняется.
- Одна транзакция (`RescheduleJob`): условное `UPDATE ... WHERE status='scheduled'`, удаление старых элементов, вставка новых. Если запуск успел стартовать или отменён — 400. Пустое время запускает рассылку сразу (`running`).
- Воркер стартует запуск условным `StartDueJob` (статус всё ещё `scheduled` и время пришло): отмена или перенос во время тика его не запустят.
- Успех: 303 на `/campaigns/{campaign_id}`, в аудит пишется `reschedule`.
- `POST /jobs/{id}/cancel` принимает поле `return`: учитываются только локальные пути `/campaigns/...`, иначе возврат на `/jobs/{id}`.

На странице кампании каждый запланированный запуск — карточка «Запланирована на <время в часовом поясе браузера>, список, число получателей» с кнопками «Изменить» (`/campaigns/{id}/launch?job=<id>`) и «Отменить»; на странице запуска (`/jobs/{id}`) — плашка и «Изменить». Время выводит шаблон `local_time` (UTC в разметке, `app.js` переводит в местное).

## Разметка (контракт views ↔ JS ↔ CSS)

Общие куски — в `views/_campaign_flow.html` (`views.go` подключает файлы с `_` в начале к layout как
общие шаблоны и не делает из них страниц): `{{define "cf_letter"}}` (строки письма), `{{define "cf_send"}}`
(шаги запуска) и `{{define "cf_letter_data"}}` (скрытое письмо для страницы запуска).
Скрипты: `/static/js/i18n/campaign-flow.js` подключается в `layout.html` после `i18n/campaigns.js`;
`/static/js/campaign-flow.js` — в конце content всех трёх страниц, перед ним
`window.CF_CONFIG = {mode, campaignId, noPublicURL, isAdmin, userEmail, senderDomains}`
(на странице запуска ещё `editing` — правка запланированного).

```
div.cf#cf [data-mode=create|edit|launch] [data-campaign-id]
  (create) form#cf-form method=post action=/campaigns novalidate   — обёртка карточки письма и кнопок
  (launch) div[hidden] — письмо для превью (cf_letter_data), затем
           form#cf-form method=post action=/campaigns/{id}/send | /jobs/{jobID}/schedule novalidate
  h2.cf-title                       «Письмо»
  p.cf-note                         (edit) подсказка про следующие отправки
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
          button.btn [data-act=cancel]           «Отмена» (видна в edit-«Письмо»)
          button.btn.btn-primary [data-act=primary]  текст ставит JS: Далее / Готово / Сохранить
    div.cf-group                    карточка необязательных строк
      p.cf-group-h                  (create) «Можно не менять»
      section.cf-row …
  (шаг check) div.cf-foot.cf-foot-send
    p.cf-final-help#cf-send-help    почему кнопка выключена
    button#cf-send type=submit                                  (launch) текст ставит JS
  div.cf-final                      (create) под строками письма
    button#cf-save type=submit name=action value=save      «Сохранить»
    button#cf-launch type=submit name=action value=launch  «Сохранить и отправить…»
    p.cf-final-help#cf-final-help   почему кнопки выключены
```

Ошибки у полей: `small.cf-err[data-err=<имя поля>][role=alert][hidden]` сразу после поля.

Состояния строки (ставит JS): `.is-open`, `.is-locked`, `.is-done`; у точки `.cf-dot` классы
`ok` (✓), `cur` (номер, синяя), `todo` (номер, серая), `opt` (пустой кружок), `warn` (!).
Номера шагов JS проставляет сам; на странице запуска шаги нумеруются с 1.

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
  - (edit, `Content.Extra > 0`) `p.cf-note` «У рассылки ещё N версий. При сохранении они отключатся.»
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
- `check`: `div.cf-check` → `div.cf-panel.cf-wide` (итог): `dl.cf-recap#cf-recap` (заполняет JS);
  `div.cf-panel.cf-wide` (превью): `div.cf-seg#cf-dev` (`button[data-dev=pc|phone]`),
  `div.cf-seg#cf-pv-ver[hidden]` (`button[data-ver=a|b]`), `p.cf-pv-subject` → `span#cf-pv-subject`,
  `div.cf-stage#cf-stage` → `iframe.cf-preview#cf-preview[sandbox=""]` + `p.cf-stage-msg#cf-preview-msg[hidden]`;
  `div.cf-panel` (пробное письмо): `input#cf-test-to[type=email][value=UserEmail]`, `button.btn#cf-test-btn`,
  `p.cf-test-result#cf-test-result[role=status][hidden]`; `div.cf-panel` (готовность): `div.cf-ready#cf-ready`.
- `when`: `div.cf-radio-row` с `input[type=radio][name=cf_when][value=now|later]`;
  `div#cf-when-later[hidden]`: `input#scheduled_at[name=scheduled_at][type=datetime-local][disabled]`,
  `input[type=hidden]#tz[name=tz]`, `#tz_offset[name=tz_offset]`, `span#cf-tz-label`;
  `input[type=checkbox]#dry_run[name=dry_run][value=on]` (нет при правке запланированного), `div.cf-pilot#cf-pilot[hidden]` с
  `input#dry_run_limit[name=dry_run_limit][type=number][min=1][max=100][value=10][disabled]`;
  `details.cf-more` с `input[type=checkbox][name=servers][value][checked]` по серверам и `select#strategy[name=strategy]`.

Превью и пробное письмо — как в мастере (`/campaigns/wizard/preview`, `/campaigns/wizard/test`),
ширина 600 / 375, переключатель A/B, готовность: ссылка отписки во всех выбранных шаблонах,
проксирующий домен, домен отправителя (известен / развёрнут / DKIM).

## Переводы

- Статичный текст разметки — ключи `cmp.cf_*` в `static/js/i18n/campaigns.js` (en и ru).
- Текст, который собирает JS, — ключи `cf.*` в новом `static/js/i18n/campaign-flow.js` (en и ru);
  JS может брать и существующие `cmp.wz_*`, `trk.*`.
- Старые ключи пока не удаляются; неиспользуемые вычищаются в конце. Заголовок страницы запуска —
  `cmp.launch_new` (с `data-i18n-arg-name`) и `cmp.launch_edit`; кнопки мастера — `cmp.wz_save`, `cmp.wz_save_launch`.

## Порядок работ

Параллельно: сервер (handlers, repository, server.go, тесты), разметка (views, views.go, layout, `cmp.cf_*`),
скрипт (`campaign-flow.js` вместо `campaign-wizard.js`, `i18n/campaign-flow.js`), стили (`cf-*` вместо `wz-*`).
Потом сборка, тесты, проверка в панели (`make docker-up`, localhost:8088).
