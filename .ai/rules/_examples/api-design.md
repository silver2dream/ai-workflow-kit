# (Example Rule Pack) API Design & Conventions (STRICT)

This is an optional example rule pack for AWK users.
To enable it: copy this file to `.ai/rules/api-design.md`, then add `api-design` under `rules.custom` in `.ai/config/workflow.yaml`.

Role: Senior API Architect.
Goal: Design consistent, well-documented APIs with predictable conventions for URL structure, error handling, versioning, and pagination.

This document is the source of truth for API design. All new endpoints MUST comply with these rules.

---

## 0) REST Conventions (STRICT)

### 0.1 Resource Naming
- URLs MUST use plural nouns for collections: `/users`, `/orders`, `/products`
- URLs MUST use kebab-case for multi-word resources: `/order-items`, `/user-profiles`
- Do NOT use verbs in URLs (actions are expressed via HTTP methods):
  - `/users` (correct)
  - `/getUsers` (forbidden)
- Nested resources for direct parent-child relationships only (max 2 levels):
  - `/users/{id}/orders` (correct)
  - `/users/{id}/orders/{orderId}/items/{itemId}` (too deep; flatten)

### 0.2 Resource Identifiers
- Use UUIDs or opaque string IDs in URLs; never expose auto-increment integers.
- IDs in paths MUST be validated before any DB lookup.

---

## 1) HTTP Methods (MUST)

| Method | Purpose | Idempotent | Request Body |
|--------|---------|------------|--------------|
| GET | Read resource(s) | Yes | No |
| POST | Create resource | No | Yes |
| PUT | Full replace | Yes | Yes |
| PATCH | Partial update | No | Yes |
| DELETE | Remove resource | Yes | No |

Rules:
- GET MUST NOT have side effects (no writes, no state changes).
- POST for creation MUST return `201 Created` with the created resource and `Location` header.
- PUT MUST replace the entire resource; use PATCH for partial updates.
- DELETE MUST return `204 No Content` on success; deleting a non-existent resource returns `404`.

---

## 2) Error Responses (STRICT)

### 2.1 Standard Error Format
All error responses MUST use this JSON structure:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Human-readable description",
    "details": [
      { "field": "email", "reason": "must be a valid email address" }
    ]
  }
}
```

### 2.2 Error Code Rules
- `code` MUST be a SCREAMING_SNAKE_CASE machine-readable string.
- `message` is for developers; do NOT expose internal details (stack traces, SQL).
- `details` is optional; use for field-level validation errors.

### 2.3 HTTP Status Mapping
- `400` Bad Request: malformed input, validation failure
- `401` Unauthorized: missing or invalid authentication
- `403` Forbidden: authenticated but insufficient permissions
- `404` Not Found: resource does not exist
- `409` Conflict: duplicate key, state conflict
- `422` Unprocessable Entity: semantically invalid (business rule violation)
- `429` Too Many Requests: rate limit exceeded (include `Retry-After` header)
- `500` Internal Server Error: unexpected failures (log details server-side)

Do NOT return `200` with an error payload. Use proper HTTP status codes.

---

## 3) Versioning (REQUIRED)

### 3.1 Strategy
- Use URL path versioning: `/api/v1/users`, `/api/v2/users`
- Do NOT use header-based versioning unless explicitly agreed upon.

### 3.2 Compatibility Rules
- Minor changes within a version MUST be backward-compatible:
  - Adding new optional fields to responses is allowed.
  - Adding new optional query parameters is allowed.
  - Removing or renaming fields is a BREAKING change and requires a new version.
- Deprecation: old versions MUST return `Sunset` and `Deprecation` headers for at least 90 days before removal.

---

## 4) Pagination (MUST for collections)

### 4.1 Cursor-Based (PREFERRED)
```
GET /api/v1/orders?cursor=eyJpZCI6MTIzfQ&limit=20
```

Response MUST include:
```json
{
  "data": [...],
  "pagination": {
    "next_cursor": "eyJpZCI6MTQzfQ",
    "has_more": true
  }
}
```

### 4.2 Offset-Based (ACCEPTABLE for small datasets)
```
GET /api/v1/products?offset=0&limit=20
```

Response MUST include:
```json
{
  "data": [...],
  "pagination": {
    "offset": 0,
    "limit": 20,
    "total": 142
  }
}
```

### 4.3 Rules
- Default `limit` MUST be defined (e.g., 20).
- Maximum `limit` MUST be enforced (e.g., 100).
- Requests exceeding max limit MUST return `400`, not silently cap.

---

## 5) Request/Response Conventions (STRICT)

### 5.1 Request Bodies
- Use `camelCase` for JSON field names (or `snake_case` if the project already uses it; be consistent).
- Required fields MUST be validated; return `400` with field-level details on failure.
- Unknown fields SHOULD be ignored (do NOT fail on extra fields unless security-sensitive).

### 5.2 Response Envelope
Successful responses MUST use a consistent envelope:
```json
{
  "data": { ... }
}
```

Collection responses:
```json
{
  "data": [ ... ],
  "pagination": { ... }
}
```

### 5.3 Timestamps
- All timestamps MUST be ISO 8601 format in UTC: `2024-01-15T09:30:00Z`
- Field names: `created_at`, `updated_at`, `deleted_at`

---

## 6) Documentation (REQUIRED)

### 6.1 OpenAPI Specification
- Every endpoint MUST have an OpenAPI (Swagger) spec.
- Spec MUST include: summary, description, request/response schemas, error codes.
- Spec MUST be kept in sync with implementation (generate from code or validate in CI).

### 6.2 Examples
- Every request/response schema MUST include at least one example.
- Error responses MUST include examples for each documented status code.

---

## 7) Output Format (when implementing APIs)

When producing API changes, ALWAYS include:
1) Endpoint list (method, URL, description)
2) Request/response schemas (JSON examples)
3) Error codes and scenarios
4) Notes:
   - breaking changes (if any)
   - pagination strategy
   - rate limiting considerations
5) Verification steps (curl examples or test commands)

---

## 8) Definition of Done (Checklist)

- [ ] URLs use plural nouns, kebab-case, no verbs
- [ ] Correct HTTP methods and status codes
- [ ] Error responses follow standard format with machine-readable codes
- [ ] Pagination implemented for all collection endpoints
- [ ] API version included in URL path
- [ ] OpenAPI spec updated and matches implementation
- [ ] Request validation returns field-level error details
- [ ] No internal details leaked in error messages
