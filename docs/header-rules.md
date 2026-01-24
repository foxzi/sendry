# Header Rules Guide

Sendry supports header manipulation rules to modify email headers before sending. This is useful for:

- Removing headers that expose internal infrastructure
- Adding custom tracking or compliance headers
- Standardizing headers across all outgoing mail

## Configuration

Header rules are configured in the `header_rules` section of the config file. No additional setup is required.

```yaml
header_rules:
  # Global rules (applied to all domains)
  global:
    - action: remove
      headers:
        - "X-Originating-IP"
        - "X-Mailer"
        - "User-Agent"
    - action: add
      header: "X-Processed-By"
      value: "Sendry"

  # Per-domain rules
  domains:
    example.com:
      - action: replace
        header: "X-Mailer"
        value: "Example Mail Server"
```

## Actions

### Remove

Removes specified headers from the email.

```yaml
- action: remove
  headers:
    - "X-Originating-IP"
    - "X-Mailer"
    - "User-Agent"
```

Header names are case-insensitive. All occurrences of the specified headers are removed.

### Replace

Replaces the value of a header. If the header doesn't exist, it's added.

```yaml
- action: replace
  header: "X-Mailer"
  value: "MyMailServer/1.0"
```

Only the first occurrence is replaced if multiple headers with the same name exist.

### Add

Adds a new header. Always appends, even if the header already exists.

```yaml
- action: add
  header: "X-Processed-By"
  value: "Sendry"
```

## Rule Order

1. Global rules are applied first
2. Domain-specific rules are applied after global rules
3. Rules are applied in the order they appear in the config

## Common Use Cases

### Privacy Protection

Remove headers that reveal internal infrastructure:

```yaml
header_rules:
  global:
    - action: remove
      headers:
        - "X-Originating-IP"
        - "X-Mailer"
        - "User-Agent"
        - "X-MimeOLE"
```

### Compliance Headers

Add required compliance or tracking headers:

```yaml
header_rules:
  global:
    - action: add
      header: "X-Organization"
      value: "Example Corp"
    - action: add
      header: "X-Compliance-ID"
      value: "GDPR-2024"
```

### Per-Domain Branding

Different headers for different sending domains:

```yaml
header_rules:
  domains:
    marketing.example.com:
      - action: add
        header: "X-Campaign-Source"
        value: "marketing"
    support.example.com:
      - action: add
        header: "X-Department"
        value: "support"
```

### Replace Default Headers

Replace auto-generated headers with custom values:

```yaml
header_rules:
  global:
    - action: replace
      header: "X-Mailer"
      value: "Sendry MTA"
```

## Notes

- Header matching is case-insensitive (`X-Mailer` matches `x-mailer`, `X-MAILER`)
- Multiline headers (with continuation) are handled correctly
- Rules do not affect the message body
- DKIM signing happens after header rules are applied
