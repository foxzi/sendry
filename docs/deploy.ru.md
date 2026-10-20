# Релиз и деплой по кнопке

[English version](deploy.md)

Как выпустить новую версию Sendry и раскатать её на прод-серверы из GitHub Actions, без ручных коммитов и без захода на серверы.

## Схема

```
Actions → «Release (button)»  ──тег vX.Y.Z──▶  «Release» (сборка deb, Docker, GitHub Release)
                                                           │
Actions → «Deploy» (version, target)  ──SSH──▶  Ansible upgrade.yml на серверах из inventory/prod.yml
```

Деплой **не запускается сам** после сборки. Это два отдельных действия: сначала выпуск версии, затем деплой выбранной версии на выбранные серверы. Так же делается откат: деплой предыдущей версии.

## Шаг 1. Выпуск версии: workflow «Release (button)»

Файл: `.github/workflows/release-button.yml`.

Actions → Release (button) → Run workflow (ветка `main`):

| Поле | Что значит |
|---|---|
| `bump` | `patch` — исправления (0.4.20 → 0.4.21); `minor` — новая возможность или изменение API между панелью и MTA (0.4.20 → 0.5.0); `major` — несовместимое изменение (→ 1.0.0) |
| `dry_run` | только показать, какой номер получится, тег не ставить |
| `skip_ci_check` | ставить тег, даже если CI для этого коммита не прошёл. По умолчанию workflow требует зелёный CI на HEAD `main` |

Что делает workflow:

1. Берёт последний тег `vX.Y.Z`, считает следующий номер.
2. Проверяет, что HEAD `main` ещё не помечен тегом и что CI для него успешен.
3. Ставит аннотированный тег на HEAD `main`, пушит его и сразу запускает обычный workflow **Release** (`release.yml`) на этом теге. Тот собирает бинарники, deb/rpm/apk, Docker-образ и публикует GitHub Release с автоматическими release notes.
4. В summary запуска пишет новый номер, список коммитов с прошлого тега и ссылки на сборку и на страницу релиза.

Ручной тег (`git tag v0.4.21 && git push origin v0.4.21`) по-прежнему работает точно так же.

### Почему сборка запускается явно

Тег пушится встроенным токеном `GITHUB_TOKEN`, а такие push-события GitHub намеренно не передаёт другим workflow (защита от бесконечных цепочек). Поэтому `release-button.yml` после пуша тега вызывает `gh workflow run release.yml --ref vX.Y.Z`, для чего в `release.yml` добавлен триггер `workflow_dispatch`. Результат тот же, что при ручном `git push origin vX.Y.Z`: сборка идёт на коммите тега, версия берётся из имени тега. Секретов для этого не нужно.

### CHANGELOG

Историю версий ведём в **GitHub Releases**: `release.yml` генерирует release notes из PR и коммитов. Файл `CHANGELOG.md` workflow не трогает. Если хочется записи в нём, их добавляют обычным коммитом в `main` до нажатия кнопки.

## Шаг 2. Деплой: workflow «Deploy»

Файл: `.github/workflows/deploy.yml`.

Дождаться, пока workflow **Release** для нового тега станет зелёным (на странице релиза должны быть файлы `sendry_X.Y.Z-1_amd64.deb` и `_arm64.deb`). Затем Actions → Deploy → Run workflow:

| Поле | Что значит |
|---|---|
| `version` | версия, например `0.4.21` (можно с `v`). Должна существовать как GitHub Release с deb-пакетами, иначе workflow остановится на первом шаге |
| `target` | `all` — сначала все MTA по одному, потом панель; `mta` — только MTA; `panel` — только панель |
| `limit` | один хост из inventory, например `mx1` или `mx2`. Пусто — все хосты группы |
| `force_restart` | перезапустить сервисы, даже если нужная версия уже стоит |

Порядок: job **Deploy MTA** идёт по хостам группы `sendry_mta` строго по одному (`serial: 1`), панель переживает это за счёт failover в `multi_send`. Job **Deploy panel** запускается только после успешного MTA-job (или сразу, если `target=panel`). Два деплоя одновременно не запустятся (`concurrency: deploy-production`). Секреты берутся из обычных секретов репозитория; environment с обязательным подтверждением не используется, его при желании может добавить владелец репозитория (см. ниже).

Что происходит на каждом сервере (playbook `ansible/playbooks/upgrade.yml`):

1. Читает установленную версию пакета (`dpkg-query`) и список сервисов `sendry` / `sendry-web`, которые **активны или включены** в systemd. Только они будут перезапущены и проверены. Сервис, которого нет на хосте, не трогается.
2. Проверяет, что сервисы запускаются из `/usr/bin/…`. Если unit переопределён на другой бинарник (как было с `/usr/local/bin/sendry-web` на панели), playbook останавливается с понятным сообщением: иначе пакет обновится, а процесс продолжит работать на старом бинарнике.
3. На панели перед установкой делает бэкап SQLite: `sqlite3 app.db ".backup /var/lib/sendry-web/backups/app-<старая версия>-<дата>.db"`. Хранятся 5 последних. Нужен, потому что `sendry-web serve` мигрирует базу при старте, а отката миграций нет.
4. Если версия уже стоит, установку пропускает (повторный запуск идемпотентен и ничего не рестартит). Иначе скачивает deb из GitHub Release и ставит через dpkg. Понижение версии разрешено.
5. Перезапускает сервисы из п.1 и ждёт до 30 секунд ответа `200` от `/health` (MTA: `http://127.0.0.1:8080/health`, панель: `http://127.0.0.1:8088/health`, адреса переопределяются в inventory).
6. В конце выводит `sendry version` и `sendry-web version`.

Конфиги `/etc/sendry/config.yaml` и `/etc/sendry/web.yaml` playbook **не читает и не меняет**.

## Откат

Actions → Deploy → `version` = предыдущая версия (например `0.4.20`), `target` = что откатываем. Пакет будет понижен, сервисы перезапущены.

База панели: если новая версия успела мигрировать схему и старый бинарник на ней не стартует, восстановить бэкап вручную:

```bash
sudo systemctl stop sendry-web
sudo ls -l /var/lib/sendry-web/backups/
sudo cp /var/lib/sendry-web/backups/app-0.4.20-1-<дата>.db /var/lib/sendry-web/app.db
sudo chown sendry:sendry /var/lib/sendry-web/app.db
sudo systemctl start sendry-web
```

## Серверы: inventory `ansible/inventory/prod.yml`

| Имя в inventory | Группа | Хост |
|---|---|---|
| `mx1` | `sendry_mta` | `mx1.semena.run` (77.42.38.2) |
| `mx2` | `sendry_mta` | `mx2.semena.run` (49.13.12.61) |
| `panel` | `sendry_panel` | `sendry.soroka.app` |

Файл в репозитории, секретов в нём нет. SSH-пользователь задаётся переменной `ansible_user` в конце файла (сейчас `deploy`, с `become`).

**Добавить новый MTA:** строка в `sendry_mta` в `prod.yml` плюс публичный ключ деплоя на сервере (см. ниже). Если на новом сервере `sendry` ещё не настроен, деплой просто поставит пакет и ничего не запустит.

## Разовая настройка

### Секреты GitHub

Репозиторий → Settings → Secrets and variables → Actions → New repository secret. Коллаборатору с правом записи это доступно, владелец не нужен.

| Секрет | Что это |
|---|---|
| `DEPLOY_SSH_KEY` | приватный ключ ed25519 пользователя деплоя. Сгенерировать на ноутбуке: `ssh-keygen -t ed25519 -f sendry-deploy -C sendry-deploy -N ""`, в секрет положить содержимое `sendry-deploy` целиком |
| `DEPLOY_KNOWN_HOSTS` | необязательно. Вывод `ssh-keyscan mx1.semena.run mx2.semena.run sendry.soroka.app`. Если задан, SSH проверяет отпечатки хостов; если нет, workflow пишет предупреждение и работает без проверки |

Кнопке релиза секреты не нужны.

Если владелец захочет подтверждать каждый деплой вручную, он может создать environment `production`, перенести секреты туда, включить Required reviewers и добавить `environment: production` в оба deploy-job в `deploy.yml`.

### Пользователь `deploy` на каждом сервере

```bash
sudo useradd -m -s /bin/bash deploy
sudo install -d -m 700 -o deploy -g deploy /home/deploy/.ssh
echo '<содержимое sendry-deploy.pub>' | sudo tee /home/deploy/.ssh/authorized_keys
sudo chmod 600 /home/deploy/.ssh/authorized_keys && sudo chown deploy:deploy /home/deploy/.ssh/authorized_keys
echo 'deploy ALL=(ALL) NOPASSWD: ALL' | sudo tee /etc/sudoers.d/deploy
sudo chmod 440 /etc/sudoers.d/deploy
```

Проверить с ноутбука: `ssh -i sendry-deploy deploy@mx1.semena.run sudo -n true`.

Если заходить под `root` удобнее, достаточно положить ключ в `/root/.ssh/authorized_keys` и поменять `ansible_user: root` в `prod.yml`.

### Панель перед первым деплоем

Из плана `ci-cd-plan.ru.md`, раздел 4:

- `systemctl cat sendry-web | grep ExecStart` должен указывать на `/usr/bin/sendry-web`. Если стоит override на `/usr/local/bin/sendry-web`, убрать его, удалить лишний бинарник, `systemctl daemon-reload`. Playbook откажется обновлять панель, пока override на месте.
- Решить, нужен ли локальный `sendry` на панели: если в `web.yaml` среди `servers` нет `localhost`/`127.0.0.1:8080`, выполнить `systemctl disable --now sendry`. Иначе он будет обновляться вместе с панелью, это тоже нормально.

### Первый прогон вручную

Перед тем как доверять кнопке, прогнать playbook с ноутбука (нужен `pip install ansible-core`):

```bash
cd ansible
ansible-playbook -i inventory/prod.yml playbooks/upgrade.yml -l mx1 -e sendry_version=0.4.20 --private-key ~/sendry-deploy
```

Ожидаем: пакет обновился, `systemctl is-active sendry`, `/health` отвечает, в панели в разделе Servers mx1 зелёный с новой версией. Повторный запуск той же команды ничего не меняет. Потом `-l mx2`, потом `-l sendry_panel`.

## Если что-то не так

| Симптом | Что смотреть |
|---|---|
| Release (button) падает на «CI has not passed» | дождаться зелёного CI на `main` или поставить `skip_ci_check` |
| Тег появился, а сборки нет | шаг «Start Release workflow» упал; запустить Actions → Release → Run workflow, выбрав тег как ref |
| Deploy падает на «Release vX has no …deb asset» | сборка ещё идёт или упала; дождаться workflow Release |
| «Permission denied (publickey)» | ключ в `DEPLOY_SSH_KEY` не совпадает с `authorized_keys` пользователя `deploy`, или `ansible_user` в `prod.yml` не тот |
| «runs /usr/local/bin/… instead of /usr/bin/…» | убрать override systemd-юнита на сервере, см. раздел про панель |
| Health check не дождался 200 | `journalctl -u sendry -n 50` (или `-u sendry-web`) на сервере; откатить деплоем предыдущей версии |
