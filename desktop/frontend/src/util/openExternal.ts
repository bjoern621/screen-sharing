import { BrowserOpenURL } from "../../wailsjs/runtime/runtime";

/**
 * Opens a URL in the user's default browser via the Wails runtime. Isolates the
 * one runtime helper components need so they never import the binding surface
 * directly.
 */
export function openExternal(url: string): void {
    BrowserOpenURL(url);
}
