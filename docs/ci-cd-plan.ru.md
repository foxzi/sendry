# CI/CD для Sendry: автодеплой по релизному тегу

Статус: **работает** с 2026-10-20. PR #7 смержен, секрет добавлен, первый релиз кнопкой (v0.4.21) и первый деплой кнопкой (`target=all`) прошли: mx1, mx2, mx3 и панель на 0.4.21, бэкап базы панели создан, автозапуск на mx2 сохранился (проверка исправленного playbook). Открытое: объединить `release-button.yml` и `release.yml` в один workflow с полем `bump` (решено 2026-10-20, делается отдельной веткой); DNS для mx3 и раскатка доменов на него (`new-mta-server.ru.md`, шаги 8–10); на mx1 и панели `sendry.service` остаётся `disabled` по решению пользователя. Рабочая инструкция: [deploy.ru.md](deploy.ru.md). Дата плана: 2026-10-19.

### Решения по открытым вопросам (2026-10-20)

- **Автодеплой после сборки:** нет, только кнопка. `deploy.yml` запускается вручную с выбором версии и цели. Автозапуск по `workflow_run` не добавлен; если понадобится, это отдельный триггер в `deploy.yml`, который берёт тег из `github.event.workflow_run.head_branch`.
- **CHANGELOG:** переходим на release notes GitHub (`release.yml` уже генерирует их). Workflow `CHANGELOG.md` не трогает.
- **Тег → сборка:** без PAT (решено 2026-10-20: fine-grained token на чужой личный репозиторий не выписать, classic token слишком широкий). Тег пушится `GITHUB_TOKEN`, затем workflow запускает `release.yml` через `gh workflow run --ref <tag>`; для этого в `release.yml` добавлен `workflow_dispatch`.
- **SSH-пользователь:** в `prod.yml` записан `deploy` с `become`, как в п.4 плана. Не подтверждён, меняется одной строкой (`ansible_user`).
- **Рестарт сервисов:** playbook перезапускает сервисы, которые активны **или** включены, а не только включённые: на mx1 `sendry.service` активен, но `is-enabled: disabled`, и иначе он остался бы на старом бинарнике.
- **Защита от override юнита:** если `ExecStart` сервиса указывает не на `/usr/bin/…` (случай `/usr/local/bin/sendry-web` на панели), playbook падает с сообщением, а не делает вид, что обновил.

### Что осталось сделать руками

1. ~~Добавить секрет репозитория `DEPLOY_SSH_KEY`~~ добавлен (подтверждено 2026-10-20), `DEPLOY_KNOWN_HOSTS` не задан. Коллаборатор может добавлять секреты репозитория, но не видит Environments (проверено 2026-10-20), поэтому `environment: production` из `deploy.yml` убран.
2. ~~Завести пользователя `deploy` с ключом на mx1, mx2 и панели~~ сделано 2026-10-20 на всех четырёх хостах: панель, mx1, mx2 и новый mx3 (`sendry-marketing.semena.run`, добавлен в `prod.yml`). Уточнение: mx2 оказался рабочим MTA на 0.4.15, чистый сервер это mx3.
3. ~~На панели убрать override `sendry-web`~~ проверено 2026-10-20: `ExecStart` уже `/usr/bin/sendry-web`, менять нечего. Остаётся удалить лишний файл `/usr/local/bin/sendry-web` и решить судьбу локального `sendry`.
4. Первый прогон `upgrade.yml` вручную. Сделано 2026-10-20 на панели: холостой прогон с 0.4.19 дал `changed=0`, затем обновление 0.4.19 → 0.4.20 прошло (бэкап базы создан, оба сервиса перезапущены, health ответил). Осталось: mx1, mx2, потом проверить кнопку Deploy.
   - mx1 2026-10-20: `systemctl enable sendry` сделан, холостой прогон с 0.4.15 дал `changed=0`. Обновление до 0.4.20 отложено по решению пользователя.
   - mx2 2026-10-20: проверен только чтением: пакет 0.4.15, сервис enabled, конфиг валиден для 0.4.15, 118 строк в `domains.yaml`. Не обновлялся.
   - mx3 2026-10-20: playbook поставил 0.4.20 на чистый сервер (`services: none`), дальше по `new-mta-server.ru.md` шаги 2–7 сделаны: конфиг, ufw, сервис запущен и enabled, в панели online. Ждёт DNS (AAAA, PTR v4/v6, SPF), домены не раскатаны.
5. Отключить на панели локальный `sendry`: в `web.yaml` он не используется, сервис активен, но `disabled`. Команда `systemctl disable --now sendry` на панели, после подтверждения.

### Найдено по пути (не блокирует деплой)

- **Пакет снимает автозапуск при обновлении.** `packaging/preremove.sh` выполняется не только при удалении, но и при обновлении (dpkg вызывает prerm старого пакета с аргументом `upgrade`), и делает `systemctl stop` + `systemctl disable sendry`. Поэтому после каждого обновления сервис оказывался `disabled`: так было на mx1 и на панели. Обнаружено 2026-10-20 после первого деплоя кнопкой на mx1 (`enabled` → `disabled`). Исправление в двух местах: `preremove.sh` выходит при `upgrade`/`1`, а `upgrade.yml` после установки возвращает `enabled` тем сервисам, у которых он был. Правка prerm подействует только начиная с обновления **с** версии, где она есть (при обновлении выполняется prerm старого пакета), поэтому страховка в playbook нужна в любом случае.
- ~~`/health` у MTA всегда отвечает `"version":"0.3.2"`~~ исправлено в ветке `fix/upgrade-keep-enabled` (войдёт в 0.4.21): новый пакет `internal/buildinfo`, `cmd/sendry` кладёт туда версию из ldflags, `/health` отдаёт её. Панель теперь показывает версию в списке Servers и на странице сервера, так что результат деплоя виден из панели.
- ~~Страница сервера в панели всегда показывает бейдж «Online»~~ исправлено там же: бейдж берёт `Online` из `GetServerStatus`, рядом версия и uptime.
- Список MTA у панели живёт только в `web.yaml` и читается при старте. Новый сервер добавляется правкой файла и рестартом `sendry-web`. Кандидаты на потом: хранить серверы в базе панели с управлением из UI, либо перенести конфиги панели и MTA в Ansible с Vault (см. «Что сознательно не делаем сейчас»).
- **Рассылки из панели целиком падают в `failed`** (оба задания, 21.04 и 22.09.2026, mx1 и mx2). Серверы получателей отвечают `555 5.5.2 Syntax error` / `501 Invalid MAIL FROM address` на `MAIL FROM` и `550 5.1.3 ... Local mailbox <Семяныч is unavailable` на `RCPT TO`. Причина: у кампании задано имя отправителя, панель шлёт в API `from: "Семяныч семена <shop@semyanich.im>"` (это корректно), а MTA клал эту строку целиком в SMTP-конверт. Транзакционные письма магазина не страдали, потому что идут с голым адресом. Исправлено в ветке `fix/envelope-from-display-name` (`internal/api/handlers.go`, `templates.go`): в конверт идут только адреса, имена в `From`/`To`/`Cc` и `Subject` кодируются по RFC 2047 (раньше кириллица шла в заголовки сырой), `/send/template` теперь проверяет адрес отправителя. Проверить после деплоя тестовой рассылкой на свои адреса.
- **Письма висят в `deferred`, если у сервера получателя битый сертификат.** Тестовая рассылка 20.10.2026 (после выхода 0.4.22 на mx1 и mx2): письмо на mail.ru доставлено, письмо на `2marci@emalupe.com` осталось в очереди mx1. У MX `in.mail.tm` просрочен сертификат. MTA пробовал STARTTLS, рукопожатие падало, в лог писалось «continuing without encryption», но после неудачного рукопожатия соединение уже не работает, и `MAIL FROM` падал с той же x509-ошибкой. Так повторялось бы при каждой попытке, пока письмо не ушло бы в DLQ. Исправлено в ветке `fix/starttls-fallback` (`internal/smtp/client.go`): подключение и HELO вынесены в `connect`. Если STARTTLS не удался, MTA закрывает соединение, заново подключается к тому же MX и отправляет письмо без TLS (в логе `STARTTLS failed, retrying without TLS`). Тест `TestSendToMXFallsBackWhenSTARTTLSFails` поднимает фейковый SMTP-сервер с самоподписанным сертификатом; без исправления он падает с той же ошибкой, что была на проде. После деплоя письмо на emalupe должно уйти при очередном повторе.
- **Панель не видела отложенные письма.** MTA отдаёт `last_error` и для `deferred`, но воркер панели (`mapSendryStatus`) статуса `deferred` не знал, а ошибку копировал только при `failed`. Письмо вроде emalupe часами висело в панели как обычное `queued`. Исправлено в `fix/starttls-fallback`: `deferred` и `sending` → `queued`, последняя ошибка сохраняется и для `queued`, на странице рассылки она видна в колонке Retrying блока Delivery Errors (типы ошибок по §13 ТЗ, см. `sendry-web.ru.md`).
- **Статусы рассылки переставали обновляться.** Воркер каждые 5 с проверял у MTA только 20 самых старых `queued`-писем. Отложенные письма висят часами и занимали эти места, поэтому при 20+ отложенных статусы остальных писем рассылки не обновлялись, даже если те давно доставлены. Исправлено там же: воркер идёт по очереди курсором (rowid) и по кругу проверяет все `queued`-письма. Пропускная способность прежняя (20 проверок за цикл): на рассылке в 10 000 писем полный круг займёт около 40 минут.
- **Список писем рассылки падал, пока есть `pending`.** У новых записей `send_job_items` поля `error` и `sendry_msg_id` равны NULL, а `ListItems` читал их в обычные строки: страница Job Items отдавала 500, «Recent Items» на странице рассылки был пуст. Исправлено там же (`COALESCE`).


## Контекст

Проект уже имеет полный CI: тесты и сборка на push/PR (`.github/workflows/ci.yml`), сборка бинарников, deb/rpm/apk пакетов и Docker-образа по тегу `v*` (`release.yml`), публикация APT/YUM репозитория на GitHub Pages (`publish-packages.yml`). Есть Ansible-роль `ansible/roles/sendry` с переключателями `sendry_mta_enabled` / `sendry_web_enabled`.

Чего нет: автоматической раскатки релиза на реальные серверы. Сейчас серверы обновляются вручную и версии разъехались:

| Сервер | Роль | Пакет sendry | Примечание |
|---|---|---|---|
| `sendry` (панель) | sendry-web + локальный sendry | 0.4.19 | `sendry-web` запущен из `/usr/local/bin`, не из пакета |
| `sendry-mx1` (+ другие MTA) | sendry | 0.4.15 | только `sendry.service` |

Последний релиз: v0.4.20. Конфиги `config.yaml` и `web.yaml` на серверах правлены вручную.

### Что уже есть в CI/CD

**CI на push/PR в main** (`.github/workflows/ci.yml`): тесты с `-race`, `go vet`, сборка только `sendry` (linux/darwin × amd64/arm64). `sendry-web` в CI не собирается. Линтера нет (`make lint` есть, конфига golangci-lint и шага в CI нет).

**Release по тегу `v*`** (`.github/workflows/release.yml`):
1. `build-binaries` — `sendry` под 4 платформы, версия из тега.
2. `build-web` — `sendry-web` linux amd64/arm64, статическая сборка с CGO в контейнере golang:alpine, arm64 через QEMU.
3. `build-packages` — nfpm: deb/rpm/apk. Один пакет `sendry` с обоими бинарниками, двумя systemd-юнитами и примерами конфигов. Postinstall создаёт `config.yaml` из примера и делает daemon-reload, сервисы не стартует.
4. `build-docker` — один мультиарх образ `ghcr.io/foxzi/sendry` (alpine, оба бинарника), теги `X.Y.Z`, `X.Y`, `X`, `latest`.
5. `release` — GitHub Release с checksums и автогенерированными notes, `-rc/-beta/-alpha` → prerelease; затем `gh workflow run publish-packages.yml`.

**Publish Packages** (`.github/workflows/publish-packages.yml`): по событию release или вручную. Скачивает пакеты из Release, собирает APT/YUM/APK репозитории (APT подписывается GPG при наличии секрета), публикует на GitHub Pages. Отсюда Ansible-роль ставит пакеты в режиме `repository`.

**Ansible** (`ansible/`): роль ставит пакет (repository / package / binary), генерирует `config.yaml` и `web.yaml` из шаблонов, разворачивает DKIM, опционально Caddy и firewall. Molecule-тесты есть, в CI не запускаются. Рассчитана на ручной запуск.

**Пробелы:**
- Нет workflow, который после релиза заходит на серверы. Отсюда разъехавшиеся версии и ручной `sendry-web` в `/usr/local/bin` на панели.
- Установка пакета в роли не перезапускает сервисы (`install.yml` не делает `notify: Restart`), процесс продолжает работать на старом бинарнике.
- Роль перезаписывает конфиги из шаблонов, а конфиги на серверах правлены вручную.
- Нет бэкапа SQLite панели перед обновлением, хотя `sendry-web serve` мигрирует базу при каждом старте.

### Принятые решения

- Репозиторий **не разделять**. Оба бинарника используют общий `internal/`, у панели и MTA общий API-контракт, одна версия на всё. Разделение только усложнит выпуск.
- Деплой: **GitHub-hosted runner → SSH → Ansible**, триггер — релизный тег `v*`, плюс ручной запуск любой версии (откат).
- Автодеплой **только обновляет бинарники** (пакет) и рестартит сервисы. Конфиги на серверах не трогает. Перенос конфигов в Ansible Vault — отдельная задача на потом.
- Локальный MTA на панели: проверить по `web.yaml`, нужен ли; если панель не ссылается на `localhost:8080` — отключить.

## Что будет сделано

### 1. Новый playbook `ansible/playbooks/upgrade.yml`

Лёгкий playbook, не зависящий от роли `sendry` (роль перегенерирует конфиги, нам это не нужно). Логика:

- Входные параметры: `sendry_version` (обязательный, например `0.4.20`).
- Определить arch и собрать URL deb-пакета из GitHub Release по тому же шаблону, что в `ansible/roles/sendry/tasks/install.yml` (блок «Install from GitHub package»):
  `https://github.com/foxzi/sendry/releases/download/v{ver}/sendry_{ver}-1_{arch}.deb`.
- Если установлена нужная версия (`dpkg-query -W -f='${Version}' sendry`) — пропустить установку.
- Иначе скачать и поставить через `apt: deb:`.
- **Бэкап БД панели** перед установкой: на хостах с `sendry-web` скопировать `/var/lib/sendry-web/app.db` через `sqlite3 .backup` в `/var/lib/sendry-web/backups/app-<version>-<date>.db`, хранить последние 5. Нужен, потому что `sendry-web serve` мигрирует SQLite при старте (`internal/web/server/server.go`, вызов `database.Migrate()`) и отката миграций нет.
- Перезапустить **только те сервисы, что включены на хосте**: `systemctl is-enabled sendry` / `sendry-web` → restart. Это закрывает дыру в роли, где установка пакета не вызывает рестарт.
- Health check после рестарта: `GET http://127.0.0.1:8080/health` для MTA, `GET http://127.0.0.1:8088/health` для панели (адрес переопределяется переменной хоста, т.к. `web.yaml` может слушать другой адрес). Ждать до 30 сек, иначе fail.
- Вывести `sendry version` / `sendry-web version` в конце.

Порядок хостов задаётся в workflow, а не в playbook: сначала группа MTA с `serial: 1` (по одному, панель переживает за счёт failover в `multi_send`), потом панель.

### 2. Inventory для прода: `ansible/inventory/prod.yml`

Коммитится в репо (хостнеймы MTA и так публичны в MX-записях, секретов в нём нет):

```yaml
all:
  children:
    sendry_mta:
      hosts:
        sendry-mx1: { ansible_host: <ip-или-fqdn> }
        # sendry-mx3: новый сервер, см. new-mta-server.ru.md
    sendry_panel:
      hosts:
        sendry: { ansible_host: <ip-или-fqdn> }
  vars:
    ansible_user: deploy        # отдельный пользователь, см. п.4
    ansible_become: true
```

`.gitignore` сейчас игнорирует только `hosts.yml`, `prod.yml` попадёт в репо.

### 3. Workflow `.github/workflows/deploy.yml`

Триггеры:
- `workflow_run` на завершение workflow **Release** со статусом success (тег берётся из `github.event.workflow_run.head_branch`). Деплой стартует после того, как в GitHub Release выложены deb-пакеты, APT-репозиторий ждать не нужно.
- `workflow_dispatch` с inputs: `version` (обязательный), `target` (`all` / `mta` / `panel`), `limit` (опционально, один хост).

Джобы:
1. `deploy-mta`: environment `production`, checkout, `pip install ansible`, SSH-ключ из секрета в `~/.ssh/id_ed25519`, known_hosts из секрета, затем
   `ansible-playbook -i ansible/inventory/prod.yml ansible/playbooks/upgrade.yml -l sendry_mta -e sendry_version=X`.
2. `deploy-panel`: `needs: deploy-mta`, то же для группы `sendry_panel`.

Environment `production` в настройках GitHub даёт место для секретов и опциональный required reviewer, если позже захочется ручной approve.

Секреты в GitHub: `DEPLOY_SSH_KEY` (приватный ключ ed25519), `DEPLOY_KNOWN_HOSTS`.

Concurrency group `deploy-production`, чтобы два деплоя не шли параллельно.

### 4. Разовая подготовка серверов (ручные шаги)

На всех серверах:
- Создать пользователя `deploy` с sudo без пароля (`deploy ALL=(ALL) NOPASSWD: ALL` или ограниченный список: `apt-get`, `dpkg`, `systemctl`, `sqlite3`). Положить публичный ключ из п.3.
- Проверить, что `sqlite3` есть на панели (для бэкапа), иначе `apt install sqlite3`.

На панели (`sendry`):
- Проверить `systemctl cat sendry-web | grep ExecStart`. Если unit переопределён на `/usr/local/bin/sendry-web` — убрать override (`/etc/systemd/system/sendry-web.service` или drop-in), удалить `/usr/local/bin/sendry-web`, `daemon-reload`, чтобы работал пакетный `/usr/bin/sendry-web`. Перед этим сравнить `sendry-web version` обоих бинарников.
- Проверить `grep -A3 servers: /etc/sendry/web.yaml`. Если среди `base_url` нет `localhost`/`127.0.0.1:8080` → `systemctl disable --now sendry`. Если есть — оставить, панель обновляется как MTA + web.
- `web.yaml.bak.*` и старый `sendry-web.db` не трогаем.

Первый прогон делаем вручную с ноутбука: `ansible-playbook ... -l sendry-mx1 -e sendry_version=0.4.20`, убеждаемся, потом остальные MTA, потом панель. После этого включаем workflow.

### 5. Документация

- `docs/deploy.md` + `docs/deploy.ru.md`: как работает автодеплой, как откатиться (`workflow_dispatch` с предыдущей версией), где бэкапы БД, как добавить новый MTA (строка в `prod.yml` + ключ `deploy`).
- Ссылка из `README.md` и `docs/ansible.ru.md`.

## Файлы

Новые:
- `ansible/playbooks/upgrade.yml`
- `ansible/inventory/prod.yml`
- `.github/workflows/deploy.yml`
- `docs/deploy.md`, `docs/deploy.ru.md`

Изменяемые:
- `README.md`, `docs/ansible.md`, `docs/ansible.ru.md` — ссылка на деплой.
- `CHANGELOG.md` — запись.

Не трогаем: `release.yml`, `publish-packages.yml`, роль `sendry`, `nfpm.yaml`, `Dockerfile`.

### 3а. Выпуск версии по кнопке (решено 19.10.2026)

Деплой будет по кнопке, а не автоматически по тегу. Тег должен ставиться без ручного коммита.

Workflow `.github/workflows/release-button.yml` (`workflow_dispatch`):
- input `bump` с вариантами `patch` / `minor` / `major` (semver: patch — исправления, minor — новая возможность, major — несовместимое изменение). Динамический список конкретных версий GitHub не поддерживает, номер считается в workflow.
- шаги: взять последний тег `v*`, посчитать следующий номер, вывести его в summary, создать тег на HEAD `main` через API и запушить.
- ловушка: тег, созданный `GITHUB_TOKEN`, не запускает `release.yml` по событию push. Варианты: секрет с personal access token (`RELEASE_PAT`) для push тега, либо в конце workflow дёргать `gh workflow run release.yml` с указанием тега. Выбрать завтра, предпочтительно PAT, чтобы цепочка «тег → сборка» осталась одинаковой для ручного и кнопочного тега.
- CHANGELOG: либо workflow дописывает строку и коммитит её перед тегом, либо полагаемся на автогенерируемые release notes GitHub. Решить завтра.

Workflow `deploy.yml` остаётся с `workflow_dispatch` (version, target), автозапуск по `workflow_run` можно отключить или оставить как опцию.

Учесть: панель и MTA общаются по API, поэтому изменение API это minor, и обновлять надо оба.

## Что сознательно не делаем сейчас

- Разделение deb-пакета на `sendry` и `sendry-web`: оба бинарника в одном пакете, сервисы включаются независимо, это не мешает.
- Перенос конфигов в Ansible Vault и полный IaC через роль: отдельная задача после того, как автодеплой заработает.
- Staging-окружение: один environment `production`; при необходимости добавляется второй inventory и второй job.

## Что нужно для старта реализации

1. Адреса серверов (IP или FQDN) панели и каждого MTA, SSH-пользователь.
2. Вывод проверки на панели:

```
ssh root@<panel-host> 'systemctl cat sendry-web | grep ExecStart; grep -A6 "servers:" /etc/sendry/web.yaml | sed "s/api_key:.*/api_key: ***/"; /usr/bin/sendry-web version; /usr/local/bin/sendry-web version'
```

## Проверка

1. `ansible-playbook --syntax-check` и `ansible-lint` на новый playbook (локально).
2. Ручной прогон `upgrade.yml` на `sendry-mx1` с `-e sendry_version=0.4.20`: пакет обновился с 0.4.15, `systemctl is-active sendry`, `/health` отвечает, `sendry version` показывает 0.4.20, письма через панель продолжают уходить.
3. Повторный прогон той же версии: playbook идемпотентен, ничего не ставит и не рестартит.
4. Прогон на панели: бэкап `app.db` создан, `sendry-web` поднялся из `/usr/bin`, UI открывается, в разделе Servers все MTA зелёные.
5. `workflow_dispatch` из GitHub с `version=0.4.20`, `target=all`: оба job зелёные.
6. Следующий реальный тег `v0.4.21`: Release → deploy отрабатывает автоматически, без ручных действий.
7. Откат: `workflow_dispatch` с `version=0.4.20` после 0.4.21, сервисы вернулись на старую версию (БД панели при необходимости восстанавливается из бэкапа вручную).