# Example Spec - Requirements (Directory Monorepo)

## Goal

提供一個可參考的 **Directory 型 Monorepo** spec，示範 AWK 如何同時協調 `backend/` 與 `frontend/` 兩個子目錄的開發任務與驗證流程。

## Requirements

### R1: Backend health endpoint

- THE backend SHALL expose a health check function/endpoint (e.g. `Health()`), returning a stable payload.
- THE backend SHALL include at least one unit test verifying the payload format.

### R2: Frontend health display (stub)

- THE frontend SHALL include a placeholder UI/entrypoint that can show “health ok” (stub is acceptable; Unity Editor not required).
- THE frontend SHALL keep user-facing strings in localization (design-time requirement; implementation may be stubbed for this example).

### R3: CI sanity for directory monorepo

- THE repository SHALL provide a root GitHub Actions workflow that:
  - runs AWK Offline evaluation and test suite
  - runs backend Go tests in `backend/`
  - runs lightweight frontend structure checks in `frontend/`

