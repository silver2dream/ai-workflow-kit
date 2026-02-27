# (Example Rule Pack) Security Checklist & Practices (STRICT)

This is an optional example rule pack for AWK users.
To enable it: copy this file to `.ai/rules/security-checklist.md`, then add `security-checklist` under `rules.custom` in `.ai/config/workflow.yaml`.

Role: Senior Security Engineer.
Goal: Prevent common vulnerabilities following OWASP guidelines, enforce defense-in-depth, and ensure secure defaults across the codebase.

This document is the source of truth for security practices. All code MUST comply with these rules. Security violations are P0 and block merge.

---

## 0) Input Validation (STRICT)

### 0.1 Trust Boundary
- ALL external input is untrusted: HTTP bodies, query params, headers, path params, WebSocket messages, file uploads.
- Validate at the boundary (handler/controller layer) BEFORE passing to business logic.

### 0.2 Validation Rules
- Validate type, length, format, and range for every input field.
- Use allowlists over denylists: define what IS valid, not what is invalid.
- Reject unexpected fields in strict mode (do NOT silently ignore unknown fields in security-critical endpoints).
- File uploads: validate MIME type, extension, and file size. Do NOT trust `Content-Type` header alone.

### 0.3 Sanitization
- Sanitize output, not input (see XSS Prevention below).
- Do NOT modify input silently; reject invalid input with a clear error message.

---

## 1) Authentication (MUST)

### 1.1 Password Rules
- Hash passwords with bcrypt (cost >= 12), scrypt, or Argon2id. NEVER use MD5 or SHA-256 alone.
- Enforce minimum password length (8+ characters). Do NOT set maximum length below 128.
- Do NOT store plaintext passwords anywhere (logs, error messages, debug output).

### 1.2 Token Rules
- Use short-lived access tokens (15-60 minutes).
- Use long-lived refresh tokens stored securely (httpOnly cookie or secure storage).
- Tokens MUST be cryptographically random (>= 256 bits of entropy).
- Invalidate all tokens on password change.

### 1.3 Session Management
- Regenerate session ID on login and privilege escalation.
- Set session cookies with: `HttpOnly`, `Secure`, `SameSite=Strict` (or `Lax`).
- Implement absolute session timeout (e.g., 24 hours) and idle timeout (e.g., 30 minutes).

---

## 2) Authorization (STRICT)

### 2.1 Principle of Least Privilege
- Every endpoint MUST enforce authorization checks.
- Default DENY: if no explicit permission, deny access.
- Do NOT rely on client-side authorization; server MUST re-validate.

### 2.2 Object-Level Authorization
- Every resource access MUST verify that the authenticated user owns or has permission to access the specific resource.
- Do NOT assume that knowing a resource ID implies authorization.
- Test for IDOR (Insecure Direct Object Reference) on every endpoint.

### 2.3 Role Checks
- Use role-based or attribute-based access control (RBAC/ABAC).
- Role checks MUST happen at the handler level, not deep in business logic.
- Admin endpoints MUST be on a separate route prefix with additional authentication.

---

## 3) XSS Prevention (MUST)

- Escape ALL dynamic content rendered in HTML using context-appropriate encoding.
- Use templating engines with auto-escaping enabled by default.
- Set `Content-Security-Policy` header to restrict inline scripts and external sources.
- Set `X-Content-Type-Options: nosniff` on all responses.
- Do NOT construct HTML by string concatenation with user input.
- For rich-text input, use a sanitization library with an allowlist of safe tags.

---

## 4) Injection Prevention (STRICT)

### 4.1 SQL Injection
- ALWAYS use parameterized queries or prepared statements.
- NEVER concatenate user input into SQL strings.
- ORM usage: verify that raw query methods still use parameterized inputs.
- Database users MUST have minimal required permissions (no `DROP`, `GRANT` for app users).

### 4.2 Command Injection
- NEVER pass user input to shell commands (`exec`, `system`, `os.Exec`).
- If shell execution is unavoidable, use allowlisted commands with strict argument validation.

### 4.3 NoSQL Injection
- For MongoDB/DynamoDB: validate query operators. Reject `$` prefixed keys from user input.
- Use typed query builders instead of raw JSON construction.

---

## 5) CSRF Protection (REQUIRED for state-changing operations)

- All state-changing requests (POST, PUT, PATCH, DELETE) MUST include CSRF protection.
- Use synchronizer token pattern or double-submit cookie pattern.
- Verify `Origin` and `Referer` headers as a defense-in-depth measure.
- API-only endpoints using Bearer tokens in `Authorization` header are exempt (tokens are not auto-sent by browsers).

---

## 6) Sensitive Data Handling (STRICT)

### 6.1 At Rest
- Encrypt sensitive data at rest (PII, financial data, health records).
- Use envelope encryption with key management service (KMS).
- Database backups MUST be encrypted.

### 6.2 In Transit
- Enforce TLS 1.2+ for all connections (HTTP, database, cache, message queues).
- Set `Strict-Transport-Security` header with `max-age >= 31536000`.
- Do NOT disable certificate verification in production code.

### 6.3 In Code
- NEVER hardcode secrets (API keys, passwords, tokens) in source code.
- Use environment variables or secret management systems (Vault, AWS Secrets Manager).
- Add `.env`, `credentials.json`, `*.pem`, `*.key` to `.gitignore`.
- Secrets in CI MUST use encrypted variables, not plaintext in config files.

---

## 7) Dependency Management (REQUIRED)

- Run automated dependency vulnerability scanning in CI (Dependabot, Snyk, `govulncheck`, `npm audit`).
- Critical/High vulnerabilities MUST be patched within 7 days.
- Pin dependency versions (use lock files: `go.sum`, `package-lock.json`).
- Review new dependencies before adding: check maintenance status, license, known vulnerabilities.

---

## 8) Logging & Monitoring (MUST)

### 8.1 What to Log
- Authentication events (login, logout, failed attempts, lockouts).
- Authorization failures (403 responses).
- Input validation failures.
- Administrative actions (role changes, config changes).

### 8.2 What NOT to Log
- Passwords, tokens, API keys, session IDs.
- Full credit card numbers, SSNs, or other PII.
- Request/response bodies containing sensitive fields.

### 8.3 Log Protection
- Logs MUST be append-only and tamper-evident.
- Set up alerts for: brute-force patterns, unusual error rates, privilege escalation attempts.

---

## 9) Verification (REQUIRED)

When implementing security-related changes, verify:
1) Static analysis passes (linter security rules enabled)
2) No hardcoded secrets detected (`gitleaks`, `detect-secrets`)
3) Dependency scan shows no critical vulnerabilities
4) Manual review of authentication/authorization logic

---

## 10) Definition of Done (Checklist)

- [ ] All inputs validated at trust boundary
- [ ] Authentication uses approved hashing and token strategies
- [ ] Authorization checked on every endpoint with object-level verification
- [ ] No SQL/command/NoSQL injection vectors
- [ ] XSS mitigated with auto-escaping and CSP headers
- [ ] CSRF protection on state-changing endpoints
- [ ] No secrets in source code or logs
- [ ] TLS enforced for all connections
- [ ] Dependencies scanned for vulnerabilities
- [ ] Security events logged without leaking sensitive data
