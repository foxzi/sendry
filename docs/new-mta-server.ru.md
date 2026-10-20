# Новый MTA-сервер с нуля

Пошаговая инструкция: как из чистого сервера на Ubuntu или Debian сделать ещё один MTA (как `sendry-mx1`) и подключить его к существующей панели.

Везде ниже подставляй свои значения:

| Плейсхолдер | Что это | Пример |
|---|---|---|
| `<FQDN>` | полное имя нового сервера | `mx3.example.com` |
| `<IP>` | публичный IPv4 нового сервера | `203.0.113.20` |
| `<IP_панели>` | публичный IP сервера панели | `203.0.113.10` |
| `<домен>` | домен, от имени которого шлём письма | `example.com` |
| `<селектор>` | DKIM-селектор, как в панели | `mail` |

Все команды на сервере выполняются от root или через `sudo`.

## Как устроен mx1 (снято 19.10.2026, только чтение)

| Что | Значение |
|---|---|
| ОС | Ubuntu 24.04, пакет sendry 0.4.15, процесс запущен 16.04.2026 |
| Адреса | 77.42.38.2 и 2a01:4f9:c014:d3a3::1, PTR на `mx1.semena.run` есть |
| Слушает | 25, 587, 8080. Порт 465 и TLS-сертификаты не настроены, папка certs пуста |
| Конфиг | `/etc/sendry/config.yaml`, сгенерирован Ansible-ролью, один SMTP-пользователь `admin`, секция `domains` пустая |
| Домены | 19 штук в `/var/lib/sendry/domains.yaml`, все раскатаны из панели. Селектор DKIM у всех `selector`, у `semyanich.im` — `mail` |
| Панель | ходит на API mx1 по IPv6 с адреса `2a01:4f8:c013:ddcd::1` |
| Firewall | iptables для IPv4: всё закрыто, кроме 22, 80, 443 и любого TCP с 82.196.2.30. Для IPv6 правил нет, то есть 8080 и 25/587 открыты всему IPv6-интернету |
| SPF | у всех доменов `include:_spf.semena.run`; эта запись перечисляет IP всех MTA. Добавлять новый IP нужно только в неё |
| Сервис | `sendry.service` активен, но **не включён в автозапуск** (`is-enabled: disabled`). После перезагрузки mx1 не поднимется |
| Исходящий 25 | открыт, доставка на mail.ru и другие работает |

Уточнение 20.10.2026: `mx2.semena.run` (49.13.12.61) оказался уже рабочим MTA, а не пустым сервером: пакет 0.4.15, сервис активен, домены раскатаны. Новый сервер это **mx3** = `sendry-marketing.semena.run`, 162.55.35.18 (Hetzner, Ubuntu 26.04). Состояние на 20.10.2026:

| Что | mx3 |
|---|---|
| A-запись | есть, 162.55.35.18 |
| AAAA | нет; есть ли у сервера IPv6, см. шаг 0 |
| PTR | **нет**, отдаёт `static.18.35.55.162.clients.your-server.de` |
| В `_spf.semena.run` | **нет** |
| Порты снаружи | 22, 25, 80, 443, 8080 открыты, firewall не настроен |
| Исходящий 25 | работает (проверено `nc` на gmail) |
| Пакет sendry | не установлен, `/etc/sendry` нет |
| Пользователь `deploy` | создан |

В примерах ниже новый сервер называется `mx3`.

## 0. Передать администратору DNS и хостинга

Это можно запросить сразу, параллельно с установкой. Без этих записей сервер поднимется, но боевые письма через него слать нельзя.

1. **Исходящий порт 25.** Подтвердить у хостера, что с сервера разрешены исходящие соединения на порт 25. Многие VPS блокируют его по умолчанию. Проверка с самого сервера, когда он будет доступен:
   ```
   nc -vz -w 5 gmail-smtp-in.l.google.com 25
   ```
   Должно быть `succeeded`. Если `timed out`, порт закрыт у хостера.
2. **A-запись** для имени сервера: `<FQDN>  A  <IP>`.
3. **PTR-запись** (обратный DNS): `<IP> → <FQDN>`. Настраивается в панели хостера, не в DNS домена. Без PTR Gmail и Mail.ru отклоняют письма.
4. **SPF.** Все отправляющие домены ссылаются на `_spf.semena.run`, поэтому новый IP добавляется один раз, в TXT-запись `_spf.semena.run`: дописать `ip4:<IP>` и, если есть IPv6, `ip6:<IPv6>`. Сейчас там:
   ```
   v=spf1 ip4:77.42.38.2 ip4:49.13.12.61 ip6:2a01:4f9:c014:d3a3::1 ip6:2a01:4f8:c013:2c17::1 ~all
   ```
   Проверить результат можно командой из шага 8.
5. **DKIM** менять не нужно. Ключ общий для всех MTA, TXT-запись `<селектор>._domainkey.<домен>` уже опубликована.

## 1. Установка пакета

```
echo "deb [trusted=yes] https://foxzi.github.io/sendry/apt stable main" | sudo tee /etc/apt/sources.list.d/sendry.list
sudo apt update
sudo apt install sendry=0.4.20-1
sendry version
```

Ставим ту же версию, которая будет на остальных серверах. Пакет создаёт пользователя `sendry`, каталоги `/etc/sendry` и `/var/lib/sendry`, systemd-юнит `sendry.service` и пример конфига. Сервис после установки не запущен, это нормально.

Задать имя хоста, оно попадает в SMTP-приветствие:

```
sudo hostnamectl set-hostname <FQDN>
```

## 2. Конфиг `/etc/sendry/config.yaml`

Проще всего взять конфиг с `sendry-mx1`, чтобы SMTP-пользователи, лимиты и правила заголовков совпадали. На mx1:

```
cat /etc/sendry/config.yaml
```

Скопировать содержимое в `/etc/sendry/config.yaml` на новом сервере и поменять:

| Параметр | Значение |
|---|---|
| `server.hostname` | `<FQDN>` |
| `smtp.domain` | `<FQDN>` |
| `smtp.tls.acme.domains` | `- <FQDN>` |
| `smtp.tls.acme.on_demand` | `true` |
| `api.api_key` | новый ключ, сгенерировать: `openssl rand -hex 32` |
| `api.allowed_ips` | `- <IP_панели>` и `- 2a01:4f8:c013:ddcd::1` (панель ходит на MTA по IPv6) |
| `domains` | оставить пустым, как на mx1: домены придут из панели в `domains.yaml` |

На mx1 секции `smtp.tls` нет вообще, сертификатов тоже. Для работы с панелью это не нужно, панель ходит по HTTP на 8080. Шаг 4 про TLS можно пропустить и сделать позже, если к серверу будут подключаться внешние SMTP-клиенты.

У каждого MTA свой `api.api_key`. Его потом надо будет вписать в панель, сохрани.

Права и проверка:

```
sudo chown root:sendry /etc/sendry/config.yaml
sudo chmod 640 /etc/sendry/config.yaml
sendry config validate -c /etc/sendry/config.yaml
```

Если конфига с mx1 под рукой нет, можно сгенерировать новый мастером, а потом сверить SMTP-пользователей с mx1:

```
sudo sendry init --domain <домен> --hostname <FQDN> --acme --acme-email admin@<домен> -o /etc/sendry/config.yaml --force
```

## 3. Firewall

```
sudo apt install ufw
sudo ufw allow 22/tcp
sudo ufw allow 25/tcp
sudo ufw allow from <IP_панели> to any port 8080 proto tcp
sudo ufw enable
sudo ufw status
```

Порты 587 и 465 открывать только если к этому MTA будут ходить внешние SMTP-клиенты напрямую. Панель ходит через 8080, ей они не нужны. Входящий 25 тоже не нужен: сервер только отправляет, почту для доменов принимает `mail.funtik.tech`.

Если у сервера есть IPv6, добавить и его правило, иначе повторится ситуация mx1, где по IPv6 открыто всё:

```
sudo ufw allow from 2a01:4f8:c013:ddcd::1 to any port 8080 proto tcp
```

## 4. TLS-сертификат

Нужна рабочая A-запись из шага 0. На время выпуска открыть порт 80:

```
sudo ufw allow 80/tcp
sudo sendry tls renew -c /etc/sendry/config.yaml
sudo ufw delete allow 80/tcp
sudo sendry tls status -c /etc/sendry/config.yaml
sudo chown -R sendry:sendry /var/lib/sendry/certs
```

Если A-записи ещё нет, этот шаг можно сделать позже, сервис запустится и без сертификата, но STARTTLS работать не будет.

Автообновление раз в неделю:

```
echo '0 3 * * 0 root ufw allow 80/tcp && /usr/bin/sendry tls renew -c /etc/sendry/config.yaml; ufw delete allow 80/tcp' | sudo tee /etc/cron.d/sendry-tls
```

## 5. Запуск

`enable` обязателен: на mx1 его забыли, и после перезагрузки сервис там не поднимется.

```
sudo systemctl enable --now sendry
systemctl is-enabled sendry
systemctl status sendry --no-pager
curl -s http://127.0.0.1:8080/health
journalctl -u sendry -n 30 --no-pager
```

Ожидаем `active (running)` и ответ от `/health`. Если сервис падает, причина будет в `journalctl`, чаще всего это ошибка в конфиге или занятый порт.

## 6. Подключить к панели

На сервере панели открыть `/etc/sendry/web.yaml` и добавить сервер в список:

```yaml
sendry:
  servers:
    - name: "mx1"
      base_url: "http://<IP_mx1>:8080"
      api_key: "..."
      env: "prod"
    - name: "mx3"
      base_url: "http://<IP>:8080"
      api_key: "<api_key из шага 2>"
      env: "prod"
```

Если в `multi_send` используются `weights` или `domain_affinity`, добавить `mx3` и туда. Пока DNS не готов, поставь `mx3: 0` в `weights` при стратегии `weighted_round_robin`, тогда боевые письма на него не пойдут.

Панель читает список серверов при старте:

```
sudo systemctl restart sendry-web
```

В панели открыть **Servers**: mx3 должен быть зелёным, с версией 0.4.20. Если красный, проверь с сервера панели:

```
curl -s -H "Authorization: Bearer <api_key>" http://<IP>:8080/api/v1/domains
```

Пустой список `[]` означает, что связь есть. Ошибка соединения означает firewall или неверный `allowed_ips` в конфиге MTA.

## 7. Раскатать домены и ключи из панели

Для каждого отправляющего домена: **Domains → домен → Deploy → выбрать mx3**. Панель загрузит на mx3 тот же DKIM-ключ, что стоит на mx1, и настройки домена. Затем **Templates → шаблон → Deploy → mx3** для шаблонов, которые отправляются через MTA.

Проверить на новом сервере, что файлы появились:

```
sudo cat /var/lib/sendry/domains.yaml
sudo ls -l /var/lib/sendry/dkim/<домен>/
```

Ключ должен совпадать с mx1:

```
sudo sha256sum /var/lib/sendry/dkim/<домен>/<селектор>.key
```

Та же команда на mx1 должна дать ту же сумму.

## 8. Проверить DNS

Когда админ DNS подтвердит записи из шага 0:

```
sendry dns check <домен> --all --selector <селектор>
dig +short -x <IP>
dig +short <FQDN>
```

Первая команда показывает MX, SPF, DKIM, DMARC. Вторая должна вернуть `<FQDN>`, третья `<IP>`. То же самое показывает кнопка DNS check на странице домена в панели.

## 9. Проверить доставку

Репутация IP:

```
sendry ip check <IP>
```

Тестовое письмо на свой ящик прямо с сервера:

```
sendry test send -c /etc/sendry/config.yaml --to <твой@ящик>
journalctl -u sendry -f
```

В полученном письме открыть исходник и найти заголовок `Authentication-Results`: `spf=pass`, `dkim=pass`. Для полной проверки отправить письмо на адрес с сайта mail-tester.com и посмотреть оценку, там же видно PTR.

Из панели: **Settings → Test email**, выбрать сервер mx3.

## 10. Что дальше

- Если ставил `mx3: 0` в весах, после успешных тестов поднять вес. Новый IP «холодный», первые дни лучше давать ему небольшую долю писем.
- Добавить сервер в `ansible/inventory/prod.yml` из плана CI/CD (`ci-cd-plan.ru.md`), чтобы он обновлялся вместе с остальными.
- Записать `<FQDN>`, `<IP>` и `api_key` туда, где хранятся доступы к mx1.

## Если что-то не так

| Симптом | Где смотреть |
|---|---|
| Сервис не стартует | `journalctl -u sendry -n 50`, `sendry config validate` |
| В панели сервер красный | firewall на 8080, `api.allowed_ips`, совпадение `api_key` |
| Deploy домена падает | `journalctl -u sendry` на mx3, права на `/var/lib/sendry` |
| Письма зависают в очереди | исходящий порт 25 закрыт хостером, `nc -vz host 25` |
| Письма уходят в спам | PTR, SPF с новым IP, `sendry ip check`, mail-tester |
| STARTTLS не работает | `sendry tls status`, права на `/var/lib/sendry/certs` |