# Unity Architecture & Patterns (STRICT)

Role: Senior Unity Engineer (R3 + UniTask + MessagePipe + UI Toolkit + Localization).
Goal: Implement features while strictly following the layered architecture and UIFlow pattern.

## Architecture (MUST FOLLOW)
Unity is layered:
- UI Layer: Views + ViewModels (UI Toolkit)
- Domain Layer: Business logic (Services/Models/Events)
- Infrastructure: Network/Analytics/Storage/Config/Localization
- Managers: UIManager, NetworkManager, AnalyticsManager
Events are reactive (R3) and routed via an event bus (MessagePipe) and UIFlow state machine.

## Folder/Placement Rules (STRICT)
Use the existing structure (do NOT invent new top-level folders):
- `Assets/Scripts/Core/` (entry)
- `Assets/Scripts/Framework/` (Async, EventBus, UIFlow)
- `Assets/Scripts/Managers/`
- `Assets/Scripts/Domain/<Feature>/` (Models/Services/Events)
- `Assets/Scripts/Infrastructure/<Area>/` (Network, Config, Localization, Storage, Analytics)
- `Assets/Scripts/UI/Views/` + `Assets/Scripts/UI/ViewModels/` (UI layer)

If there is mismatch between documents and repo reality:
- Prefer the repo’s existing folders.
- Only add missing folders if absolutely necessary and consistent.

## UIFlow (MUST)
- UI is a view-based finite state machine.
- State transitions are triggered by events (GameEvent) and coordinated by `UIFlowManager`.
- Views should publish events (e.g., `OnRequestBreakthrough`) rather than directly hard-navigating.

When implementing new flows:
- Extend `GameState` and `GameEvent` enums if needed.
- Add/modify switch logic in `UIFlowManager` for transitions.
- Keep transition conditions close to the Domain/Services; UI only triggers and renders.

## Reactive & Async (MUST)
- Use R3 streams for UI/flow signals.
- Use UniTask for async operations; do not block main thread.
- Dispose subscriptions on exit (`OnExit`) and/or via lifetime helpers.

## Localization (ABSOLUTE RULE)
- Never hardcode user-facing strings (buttons, hints, errors, UI labels).
- Use Unity Localization tables and keys.
- Use `LocalizedString` (or your LocalizationManager wrapper) and keep keys in the appropriate table
  (e.g., UI_Strings / Game_Content / System_Messages).

If you need a new UI string:
- Add a new localization key (do not embed literal text in code).
- Reference it via LocalizedString or the project’s localization helper.

## Config (MUST)
- Game configs live as JSON (Resources/Configs) and are loaded via `ConfigLoader` (Infrastructure).
- `ConfigLoader.InitializeAsync()` must happen early (e.g., GameEntry) before systems that depend on config.
- `ConfigService` (Domain) is the only place to expose business queries (e.g., realm name/description via localization keys).

## UI Toolkit Style Rules
- Prefer UXML + USS. Keep hierarchy flat where possible.
- Avoid unsupported USS/CSS features; use Unity-supported USS only.
- Bind data via ViewModel or explicit setup; avoid putting business logic in Views.

## Output Format (when asked to implement)
When producing changes, output:
1) File list + target paths
2) New/modified code blocks per file
3) Notes: how it integrates with UIFlow/EventBus/Localization/Config
