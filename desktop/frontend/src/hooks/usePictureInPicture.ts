import { useCallback, useRef, useState } from "react";

/**
 * The Document Picture-in-Picture API is not in the DOM lib types. Declaring the
 * slice used here keeps the hook free of `any` without pulling a global shim.
 */
interface DocumentPictureInPicture {
    requestWindow(options?: {
        width?: number;
        height?: number;
    }): Promise<Window>;
}

declare global {
    interface Window {
        documentPictureInPicture?: DocumentPictureInPicture;
    }
}

/** Clone the page's stylesheets into the PiP document so a moved node keeps its
 * Tailwind styling; the PiP window is a separate document with an empty head. */
function copyStyles(target: Window): void {
    for (const sheet of Array.from(document.styleSheets)) {
        try {
            const css = Array.from(sheet.cssRules)
                .map(rule => rule.cssText)
                .join("");
            const style = target.document.createElement("style");
            style.textContent = css;
            target.document.head.appendChild(style);
        } catch {
            // Cross-origin sheet: link it instead of reading its rules.
            if (sheet.href) {
                const link = target.document.createElement("link");
                link.rel = "stylesheet";
                link.href = sheet.href;
                target.document.head.appendChild(link);
            }
        }
    }
}

/**
 * Pops a tile's DOM out into an always-on-top Document Picture-in-Picture
 * window and back. It moves the actual node (video or canvas plus chrome), so it
 * works for every decoder, unlike the video-only element PiP. Unsupported
 * browsers (the WebKitGTK window) report supported=false so the caller hides the
 * control rather than showing a dead button.
 */
export function usePictureInPicture() {
    const supported =
        typeof window !== "undefined" && "documentPictureInPicture" in window;
    const [active, setActive] = useState(false);
    const pipWindow = useRef<Window | null>(null);

    const toggle = useCallback(
        async (node: HTMLElement) => {
            if (!supported) return;
            if (pipWindow.current) {
                pipWindow.current.close();
                return;
            }
            const parent = node.parentElement;
            const sibling = node.nextSibling;
            const win = await window.documentPictureInPicture!.requestWindow({
                width: node.clientWidth || 640,
                height: node.clientHeight || 360,
            });
            pipWindow.current = win;
            copyStyles(win);
            win.document.body.style.margin = "0";
            win.document.body.appendChild(node);
            setActive(true);
            win.addEventListener(
                "pagehide",
                () => {
                    if (parent) parent.insertBefore(node, sibling);
                    pipWindow.current = null;
                    setActive(false);
                },
                { once: true }
            );
        },
        [supported]
    );

    return { supported, active, toggle };
}
