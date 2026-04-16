# Синхронизация DNS

Команда `sendry-web dns-sync` сравнивает текущие DNS-записи домена с
рекомендациями Sendry (SPF, DKIM, DMARC) и по желанию создаёт или обновляет
их через API DNS-провайдера.

Поддерживаемые провайдеры:

- Cloudflare (`--provider cloudflare`)
- Namedot (`--provider namedot`, см. [github.com/foxzi/namedot](https://github.com/foxzi/namedot))

## Рекомендуемые записи

Для домена `example.com` с DKIM-селектором `mail` проверяются:

| Тип   | Имя                          | DNS | Ожидаемое значение                                             |
|-------|------------------------------|-----|----------------------------------------------------------------|
| SPF   | `example.com`                | TXT | `v=spf1 a mx ~all` (или с `include:<spf_include>`)             |
| DMARC | `_dmarc.example.com`         | TXT | `v=DMARC1; p=quarantine; rua=mailto:dmarc@example.com`         |
| DKIM  | `mail._domainkey.example.com`| TXT | Значение DNS-записи привязанного DKIM-ключа                    |

Часть SPF `include:` управляется глобальной переменной `spf_include`
(Настройки → Глобальные переменные). Примеры:

- `_spf.mailgun.org`
- `spf.sendgrid.net`

DKIM проверяется только если на домене включён DKIM и ключ привязан.

## Аутентификация Cloudflare

Поддерживаются два режима:

### API Token (рекомендуется)

Создайте токен в панели Cloudflare с правами:

- Zone → Zone → Read
- Zone → DNS → Edit
- Zone Resources: *Include → All zones from an account → ваш аккаунт* (или конкретные зоны)

Передайте его через `--token` или `CLOUDFLARE_API_TOKEN`. Это предпочтительный
вариант, одного токена достаточно на все домены аккаунта.

### Legacy Global API Key

Поддерживается для совместимости. Global API Key даёт полный доступ ко всему
аккаунту, поэтому scoped API Token безопаснее.

Укажите:

- `--email <email-аккаунта>` или `CLOUDFLARE_API_EMAIL`
- `--token <global-key>` или `CLOUDFLARE_API_KEY`

Режим можно форсировать флагом `--auth global` (режим `auto` сам выберет его,
если задан `--email`).

## Namedot

Namedot — собственный DNS-сервер с REST API. Провайдер использует авторизацию
по bearer-токену и REST-эндпоинты для зон и RRSet.

Обязательные параметры:

- `--namedot-url <base-url>` или `NAMEDOT_API_URL` (например, `https://dns.example.com`)
- `--token <bearer-token>` или `NAMEDOT_API_TOKEN`

Токен должен быть валидным для REST API namedot (`api_token` / `api_token_hash`
в конфиге namedot).

Пример:

```bash
sendry-web dns-sync --config /etc/sendry/web.yaml \
  --provider namedot \
  --namedot-url "$NAMEDOT_API_URL" \
  --token "$NAMEDOT_API_TOKEN" \
  --domain example.com
```

Замечания:

- TXT-значения автоматически оборачиваются в кавычки при отправке в namedot и
  разворачиваются при чтении, поэтому сравнения корректны между провайдерами.
- Обновление заменяет соответствующий RRSet в namedot на одну запись — не
  используйте команду с зонами, где на одном имени нужно хранить несколько TXT
  для SPF/DMARC/DKIM.

## Использование

Проверка одного домена (план, без изменений):

```bash
sendry-web dns-sync --config /etc/sendry/web.yaml \
  --domain example.com \
  --token "$CLOUDFLARE_API_TOKEN"
```

Применить изменения (API Token):

```bash
sendry-web dns-sync --config /etc/sendry/web.yaml \
  --domain example.com \
  --apply \
  --token "$CLOUDFLARE_API_TOKEN"
```

Применить изменения через Global API Key:

```bash
sendry-web dns-sync --config /etc/sendry/web.yaml \
  --domain example.com --apply \
  --email "$CLOUDFLARE_API_EMAIL" \
  --token "$CLOUDFLARE_API_KEY"
```

Проверить все домены:

```bash
sendry-web dns-sync --config /etc/sendry/web.yaml --all \
  --token "$CLOUDFLARE_API_TOKEN"
```

## Вывод

```
DNS sync [plan] provider=cloudflare domains=1

=== example.com ===
KIND   NAME                         ACTION  STATUS   DETAILS
SPF    example.com                  noop    planned  matches expected value
DMARC  _dmarc.example.com           update  planned  value differs from expected
DKIM   mail._domainkey.example.com  create  planned  no current record found
```

Exit-код отличный от нуля, если возникли ошибки (например, зона не
найдена, ошибка API, или DKIM-ключ не привязан).

## Замечания

- Команда никогда не удаляет записи, только создаёт или обновляет.
- TXT-значения нормализуются (снимаются кавычки, схлопываются пробелы),
  поэтому отличия в кавычках не вызывают обновления.
- Если на имени несколько TXT-записей, Sendry выбирает ту, что совпадает
  по префиксу (`v=spf1`, `v=DMARC1`, `v=DKIM1`) для обновления.
