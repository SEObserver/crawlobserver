# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in CrawlObserver, **do not open a public issue**.

Email **security@crawlobserver.com** with:

1. Description of the vulnerability
2. Steps to reproduce
3. Affected version(s) and configuration
4. Impact assessment (if you have one)

We will acknowledge your report within **48 hours** and aim to provide a fix or mitigation within **7 days** for critical issues.

## What Qualifies

- SQL/command injection
- SSRF bypasses (private IP access, DNS rebinding)
- Authentication or authorization flaws
- Sensitive data exposure (credentials, tokens in logs or responses)
- Path traversal or arbitrary file access
- Cross-site scripting (XSS) in the web UI

## What Doesn't Qualify

- Denial of service via large crawls (that's a configuration issue, not a vulnerability)
- Missing rate limiting on localhost-only endpoints
- Issues requiring physical access to the machine running CrawlObserver
- Vulnerabilities in ClickHouse itself (report those upstream)

## Disclosure

We follow coordinated disclosure. We'll work with you on a timeline and credit you in the release notes (unless you prefer to remain anonymous).

We do not offer a bug bounty program at this time.
