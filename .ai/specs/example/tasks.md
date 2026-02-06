# Example Feature - Health Check (Directory Monorepo)

Repo: backend, frontend  
Coordination: sequential  
Sync: independent

## Tasks

- [ ] 1. Backend: add health check
  - Repo: backend
  - [ ] 1.1 Add `health` implementation (function or minimal handler)
    - _Requirements: R1_
  - [ ] 1.2 Add unit tests for health payload
    - _Requirements: R1_

- [ ] 2. Frontend: show health status (stub)
  - Repo: frontend
  - _depends_on: 1_
  - [ ] 2.1 Add a minimal entrypoint/script placeholder
    - _Requirements: R2_
  - [ ] 2.2 Add localization key placeholders for UI strings
    - _Requirements: R2_

- [ ] 3. CI sanity
  - Repo: root
  - [ ] 3.1 Ensure root CI runs AWK offline + tests, backend go tests, frontend sanity
    - _Requirements: R3_

- [ ] 4. Checkpoint
  - Ensure `awkit evaluate --offline` and `go test ./...` pass.
