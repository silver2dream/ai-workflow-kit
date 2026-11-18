import { useCallback, useRef } from "react";

/**
 * useAfterClose runs what a menu's item chose once the menu is gone. A sheet,
 * dialog or tour opened while the menu is still closing would have Esc taken
 * by the closing menu, and its focus given back to the menu's button.
 *
 * later(action) keeps the action; pass onCloseAutoFocus to the menu's content.
 */
export function useAfterClose() {
  const chosen = useRef<(() => void) | null>(null);
  const later = useCallback((action: () => void) => {
    chosen.current = action;
  }, []);
  const onCloseAutoFocus = useCallback((event: Event) => {
    const action = chosen.current;
    chosen.current = null;
    if (action) {
      event.preventDefault();
      action();
    }
  }, []);
  return { later, onCloseAutoFocus };
}
