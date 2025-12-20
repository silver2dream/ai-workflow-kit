# Sample Feature - Implementation Plan

Repo: backend
Coordination: sequential
Sync: independent

## Objective

This is a sample tasks.md for testing parse_tasks.py.

---

## Tasks

- [ ] 1. First main task
  - Repo: backend
  - [ ] 1.1 First subtask
    - Implementation details here
    - _Requirements: R1_
  - [ ] 1.2 Second subtask
    - More details
    - _Requirements: R1_

- [ ] 2. Second main task
  - Repo: backend
  - _depends_on: 1_
  - [ ] 2.1 Dependent subtask
    - This depends on task 1
    - _Requirements: R2_
  - [ ]* 2.2 Optional subtask
    - This is optional (note the asterisk)
    - _Requirements: R2_

- [x] 3. Completed task
  - This task is already done
  - [ ] 3.1 Subtask of completed
    - Even subtasks can be marked

- [ ] 4. Task with multiple dependencies
  - _depends_on: 1, 2_
  - [ ] 4.1 Final implementation
    - _Requirements: R3_

- [ ] 5. Checkpoint
  - Ensure tests pass. In autonomous mode, log issues and continue.
