# (Example Rule Pack) Mobile (Flutter/Dart) Architecture & Patterns (STRICT)

This is an optional example rule pack for AWK users.
To enable it: copy this file to `.ai/rules/mobile-flutter.md`, then add `mobile-flutter` under `rules.custom` in `.ai/config/workflow.yaml`.

Role: Senior Flutter Engineer.
Goal: Build cross-platform mobile apps with BLoC architecture, clean layered design, and comprehensive testing.

---

## 0) Tech Stack (assumed)

- Flutter (latest stable), Dart (null safety enforced)
- flutter_bloc / bloc for state, freezed + json_serializable for models
- dio or http for networking, go_router for navigation
- get_it for DI, mockito + bloc_test for testing

---

## 1) Project Structure (STRICT)

```
lib/
  main.dart                   # Entry: runApp, DI setup
  app.dart                    # MaterialApp / Router config
  core/
    constants.dart            # App-wide constants
    theme.dart                # ThemeData
    errors/failures.dart      # Domain failure classes
    errors/exceptions.dart    # Data-layer exceptions
  features/<feature>/
    data/
      datasources/            # Remote + local data sources
      models/                 # DTOs (freezed)
      repositories/           # Repository implementations
    domain/
      entities/               # Pure Dart domain entities
      repositories/           # Abstract repository interfaces
      usecases/               # Single-responsibility usecases
    presentation/
      bloc/                   # BLoC + Events + States
      pages/                  # Full-screen page widgets
      widgets/                # Feature-scoped widgets
  shared/widgets/             # App-wide reusable widgets
  shared/extensions/          # Dart extension methods
test/features/<feature>/      # Mirrors lib/ structure
```

### Placement rules (HARD)
- Every feature MUST follow `data / domain / presentation` split.
- Domain MUST NOT import Flutter or data-layer packages.
- Presentation depends on domain (via BLoC); never imports data layer directly.

---

## 2) BLoC Pattern (STRICT)

Each feature needs: `<feature>_bloc.dart`, `<feature>_event.dart`, `<feature>_state.dart`.

### Rules
- BLoCs depend on usecases/repos, never on data sources directly.
- BLoCs MUST NOT call `Navigator`, `context.read`, or widget APIs.
- Events and states MUST be immutable (`freezed` or sealed + `Equatable`).
- Prefer explicit state subclasses over nullable-field state bags:
  - `AuthInitial`, `AuthLoading`, `AuthAuthenticated`, `AuthError`
  - NOT: `AuthState({ bool isLoading, User? user, String? error })`
- `emit()` MUST NOT be called after async gap without checking `isClosed`.
- Each handler emits loading state, then success/failure.

---

## 3) Widget Rules (STRICT)

- Pages: `<Feature>Page`. Shared: `AppButton`, `LoadingOverlay`.
- Prefer composition over inheritance. `const` constructors wherever possible.
- `build` methods ~100 lines max; extract sub-widgets into separate classes.
- `StatelessWidget` by default. `StatefulWidget` only for local state (animation/text controllers).
- Business state in BLoCs, NOT in `setState()`. `dispose()` MUST clean up all controllers.

---

## 4) Navigation (go_router) (MUST)

- ALL routes defined declaratively in central `GoRouter` config.
- Kebab-case paths: `/user-profile`. Auth routes use `redirect` guard.
- No imperative `Navigator.push()`; use `context.go()` / `context.push()`.
- Deep linking MUST work; every route independently reachable.

---

## 5) Data Layer Rules (STRICT)

- DTOs use `freezed` + `json_serializable`. Domain entities are pure Dart.
- Model-to-entity mapping in repository implementations, not BLoCs.
- Repos return `Either<Failure, T>` or typed results; never throw to BLoC layer.
- Remote data source returns DTOs via HTTP. Local source uses SharedPreferences/Hive/SQLite.

---

## 6) Dependency Injection (MUST)

- `get_it` as service locator. All deps registered in `injection_container.dart`.
- Order: external deps -> data sources -> repos -> usecases -> BLoCs.
- BLoCs as `factory` (new per use). Repos/sources as `lazySingleton`.
- Do NOT use `get_it` in widgets; provide BLoCs via `BlocProvider`.

---

## 7) Error Handling (MUST)

- `Failure` classes in `core/errors/` (`ServerFailure`, `CacheFailure`, `NetworkFailure`).
- Repos catch exceptions and return `Failure` objects.
- BLoCs emit error states with user-readable messages mapped from `Failure`.
- Never show raw exception messages to users.

---

## 8) Testing (REQUIRED)

- Every BLoC: `bloc_test` unit tests. Every usecase: unit test with mocked repo.
- Every page: at least one widget test. Every repo: test with mocked data source.
- Use `mocktail`/`mockito`. Widget tests use `pumpWidget` with providers.
- Test state transitions, not implementation details.

---

## 9) Verification (default)

```bash
flutter analyze
flutter test
flutter build apk --debug
```

---

## 10) Definition of Done (Checklist)

- [ ] Feature follows `data / domain / presentation` split
- [ ] Domain layer has zero Flutter/package imports
- [ ] BLoC depends on usecases/repos, not data sources
- [ ] States are immutable and exhaustive
- [ ] DTOs use freezed; repos return Failures, not exceptions
- [ ] Navigation via go_router; no imperative Navigator
- [ ] DI registered in injection_container.dart
- [ ] BLoC tests + widget tests exist
- [ ] `flutter analyze` passes with zero issues
