# (Example Rule Pack) Database Migrations & Schema Changes (STRICT)

This is an optional example rule pack for AWK users.
To enable it: copy this file to `.ai/rules/database-migrations.md`, then add `database-migrations` under `rules.custom` in `.ai/config/workflow.yaml`.

Role: Senior Database Engineer.
Goal: Implement safe database migrations with zero-downtime deployments, proper rollback strategies, and data integrity guarantees.

This document is the source of truth for migration practices. All schema changes MUST follow these rules.

---

## 0) Migration Files (STRICT)

### 0.1 Location
- All migrations MUST live in a dedicated directory: `migrations/`, `db/migrations/`, or the project's established location.
- Do NOT scatter SQL/schema changes across application code.

### 0.2 One Migration Per Change
- Each migration file MUST contain exactly one logical schema change.
- Do NOT combine unrelated changes (e.g., adding a column AND creating a new table) in a single migration.
- Exception: closely related changes that MUST be atomic (e.g., table + its indexes).

### 0.3 Immutability
- Once a migration is merged to the integration branch, it MUST NOT be modified.
- Fixes to merged migrations MUST be done as new corrective migrations.

---

## 1) Naming Convention (MUST)

### 1.1 Format
Migration files MUST follow this naming pattern:
```
{timestamp}_{description}.{direction}.sql
```

Examples:
- `20240115093000_create_users_table.up.sql`
- `20240115093000_create_users_table.down.sql`
- `20240116100000_add_email_to_users.up.sql`
- `20240116100000_add_email_to_users.down.sql`

### 1.2 Timestamp
- Use UTC timestamp in `YYYYMMDDHHMMSS` format.
- Timestamps MUST be unique across all migrations.

### 1.3 Description
- Use snake_case, descriptive names.
- Prefix with the action: `create_`, `add_`, `drop_`, `rename_`, `alter_`, `migrate_data_`.

---

## 2) Forward Migrations (MUST)

### 2.1 Idempotency
- Use `IF NOT EXISTS` / `IF EXISTS` guards where the database supports them.
- Migration MUST be safe to re-run (or the tooling must track applied state).

### 2.2 Required Practices
- Every `CREATE TABLE` MUST include a primary key.
- Every foreign key MUST have an explicit `ON DELETE` clause (no implicit defaults).
- New columns with `NOT NULL` MUST have a `DEFAULT` value or be added in a multi-step migration.
- Index names MUST be explicit and descriptive: `idx_{table}_{columns}`.

### 2.3 Transactions
- Wrap each migration in a transaction where the database supports transactional DDL (PostgreSQL).
- For databases without transactional DDL (MySQL), document the manual rollback steps.

---

## 3) Rollback Migrations (REQUIRED)

### 3.1 Every Up Needs a Down
- Every forward (up) migration MUST have a corresponding rollback (down) migration.
- The down migration MUST fully reverse the up migration.

### 3.2 Data Loss Awareness
- If a rollback would cause data loss (e.g., dropping a column with data), the down migration MUST include a comment:
  ```sql
  -- WARNING: This rollback drops column 'email' and its data.
  ```
- If rollback is impossible without data loss, document this and mark the migration as `irreversible`.

### 3.3 Testing Rollbacks
- Rollback migrations MUST be tested: apply up, then down, then up again. The final state MUST match the first up.

---

## 4) Data Migrations (STRICT)

### 4.1 Separation
- Schema migrations (DDL) and data migrations (DML) MUST be in separate files.
- Data migrations MUST run after the corresponding schema migration.

### 4.2 Batching
- Large data migrations MUST be batched to avoid long-running transactions:
  - Process rows in chunks (e.g., 1000 rows per batch).
  - Include progress logging.
  - Support resume-from-failure (track last processed ID).

### 4.3 Backfill Rules
- Backfills MUST be idempotent (safe to re-run).
- Backfills MUST NOT lock tables for extended periods.
- Use `UPDATE ... WHERE id BETWEEN ? AND ?` patterns, not `UPDATE ... SET` on the entire table.

---

## 5) Zero-Downtime Rules (STRICT)

### 5.1 Forbidden Operations (without multi-step migration)
The following operations MUST NOT be done in a single migration during deployment:
- Renaming a column or table
- Changing a column type
- Adding a `NOT NULL` column without a default
- Dropping a column that the application still reads

### 5.2 Multi-Step Migration Pattern (REQUIRED)
For breaking changes, follow the expand-contract pattern:

**Step 1 (expand)**: Add new column/table alongside old one. Deploy application that writes to both.
**Step 2 (migrate)**: Backfill data from old to new.
**Step 3 (contract)**: Deploy application that only reads/writes new. Drop old column/table.

Each step is a separate migration + deployment cycle.

### 5.3 Index Safety
- Creating indexes on large tables MUST use `CONCURRENTLY` (PostgreSQL) or equivalent.
- Do NOT create indexes inside transactions when using `CONCURRENTLY`.

---

## 6) Testing Migrations (MUST)

### 6.1 CI Requirements
CI MUST verify:
1. All migrations apply cleanly from scratch (empty database to current state).
2. All migrations apply cleanly from the last release to current state.
3. All rollback migrations execute without errors.
4. The final schema matches the expected schema (schema diff check).

### 6.2 Local Development
- Developers MUST be able to run `migrate up` and `migrate down` locally.
- Seed data scripts MUST be separate from migrations.

---

## 7) Review Checklist (REQUIRED for PRs with migrations)

Before approving a PR with migrations, verify:

- [ ] Migration file follows naming convention
- [ ] Forward migration is idempotent or guarded
- [ ] Rollback migration exists and reverses the forward
- [ ] No forbidden zero-downtime operations in a single step
- [ ] Large data migrations are batched
- [ ] Indexes use `CONCURRENTLY` on large tables
- [ ] Schema and data migrations are in separate files
- [ ] Migration tested: up, down, up produces consistent state
- [ ] No hardcoded data that belongs in seed scripts

---

## 8) Output Format (when implementing migrations)

When producing migration changes, ALWAYS include:
1) Migration file list + paths
2) SQL content for both up and down files
3) Notes:
   - zero-downtime compliance (single-step or multi-step?)
   - data migration strategy (if applicable)
   - estimated impact on large tables (lock duration, index build time)
   - rollback risk assessment
4) Verification steps (apply, rollback, re-apply)
