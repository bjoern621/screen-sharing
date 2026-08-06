import { useCallback, useEffect, useState } from "react";
import {
    GridTransports,
    NativeGridRunning,
    StartNativeGrid,
    StopNativeGrid,
} from "../../wailsjs/go/app/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { PublishExit } from "../types/stream";

const POLL_INTERVAL_MS = 2000;

/**
 * Tracks the native grid window, a separate GTK binary the backend spawns.
 * running is polled, so a window closed via its own close button drops out;
 * "nativegrid:exit" carries the failure message. The window opens on the leg the
 * backend has stored for it (`gridTransport`), which its own sidebar changes, so
 * toggle names none.
 *
 * transports is the set of legs a window can open on, which is not every leg a
 * viewer can be pointed at: the grid receives through a GStreamer pipeline, so
 * it reaches WHEP and not the relay's HLS segments. An empty set means the list
 * has not arrived, and gates nothing.
 */
export function useNativeGrid() {
    const [running, setRunning] = useState(false);
    const [error, setError] = useState("");
    const [transports, setTransports] = useState<string[]>([]);

    const refresh = useCallback(async () => {
        try {
            setRunning(await NativeGridRunning());
        } catch {
            /* backend not ready yet */
        }
    }, []);

    useEffect(() => {
        const off = EventsOn("nativegrid:exit", (e: PublishExit) => {
            if (e.message) {
                setError("Native grid exited: " + e.message);
            }
            void refresh();
        });
        return () => off();
    }, [refresh]);

    useEffect(() => {
        void refresh();
        const id = setInterval(refresh, POLL_INTERVAL_MS);
        return () => clearInterval(id);
    }, [refresh]);

    useEffect(() => {
        GridTransports()
            .then(setTransports)
            .catch(() => {
                /* backend not ready yet */
            });
    }, []);

    const toggle = useCallback(async () => {
        setError("");
        try {
            if (running) {
                await StopNativeGrid();
                setRunning(false);
            } else {
                // The window receives every tile over one transport: the roster
                // the app pushes carries a source fragment per stream, built for
                // the leg the settings name. A leg with no GStreamer watch form
                // is refused by the backend, and the message lands in error.
                await StartNativeGrid();
                setRunning(true);
            }
        } catch (e) {
            setError("Native grid: " + e);
        }
    }, [running]);

    return { running, error, transports, toggle };
}
