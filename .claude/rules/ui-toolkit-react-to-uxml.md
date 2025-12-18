# UI Toolkit: React/Tailwind -> UXML/USS (STRICT)

Role: Senior Unity UI Toolkit engineer.
Task: Convert provided React/Tailwind into Unity UI Toolkit files (.uxml + .uss), and integrate into the existing Unity architecture.

## MUST Constraints
- No standard CSS assumptions. Only use properties supported by Unity USS.
  - Use `unity-text-align` where appropriate, avoid unsupported z-index/complex shadows.
- Translate Tailwind layout into Flexbox-based USS.
- Keep the UXML hierarchy flat where possible.
- DO NOT hardcode any user-facing strings:
  - Use Unity Localization (LocalizedString / keys) and integrate with existing localization approach.

## Integration Rules
- Output should include:
  - `.uxml`
  - `.uss`
  - A `View.cs` (UI layer) that loads/binds UXML
  - A `ViewModel.cs` if the UI has data/state
- Views only render + publish events; business logic lives in Domain/Services.
- If navigation/state changes are needed, publish UIFlow `GameEvent` rather than direct scene changes.
