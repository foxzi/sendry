# Синхронизация DNS

Команда `sendry-web dns-sync` сравнивает текущие DNS-записи домена с
рекомендациями Sendry (SPF, DKIM, DMARC) и по желанию создаёт или обновляет
их через API DNS-провайдера.

Поддерживаемые провайдеры:

- Cloudflare (`--provider cloudflare`)

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

## Токен Cloudflare

Создайте API-токен с правами:

- Zone → Zone → Read
- Zone → DNS → Edit

Передать токен можно через флаг `--token` или переменную окружения
`CLOUDFLARE_API_TOKEN`.

## Использование

Проверка одного домена (план, без изменений):

```bash
sendry-web dns-sync --config /etc/sendry/web.yaml \
  --domain example.com \
  --token "$CLOUDFLARE_API_TOKEN"
```

Применить изменения:

```bash
sendry-web dns-sync --config /etc/sendry/web.yaml \
  --domain example.com \
  --apply \
  --token "$CLOUDFLARE_API_TOKEN"
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
