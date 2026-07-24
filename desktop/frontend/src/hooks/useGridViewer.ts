import { useCallback, useEffect, useState } from "react";
import {
    GridViewerRunning,
    StartGridViewer,
    StopGridViewer,
} from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { PublishExit } from "../types/stream";

const POLL_INTERVAL_MS = 2000;

/**
 * Tracks the GTK grid window (cmd/gridviewer): the native viewer that decodes
 * like the gst wall but composites tiles as widgets. start() launches it over
 * the given streams, stop() closes it. running is polled, so a viewer closed
 * via its own window button is noticed too; "gridviewer:exit" carries the
 * failure message when the process died on its own.
 */
export function useGridViewer() {
    const [running, setRunning] = useState(false);
    const [error, setError] = useState("");
    const [logPath, setLogPath] = useState("");

    const refresh = useCallback(async () => {
        try {
            setRunning(await GridViewerRunning());
        } catch {
            /* backend not ready yet */
        }
    }, []);

    useEffect(() => {
        const off = EventsOn("gridviewer:exit", (e: PublishExit) => {
            if (e.message) {
                setError("GTK grid exited - " + e.message);
                setLogPath(e.logPath ?? "");
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

    const start = useCallback(async (names: string[], transport: string) => {
        setError("");
        setLogPath("");
        try {
            await StartGridViewer(names, transport);
            setRunning(true);
        } catch (e) {
            setError("GTK grid: " + e);
        }
    }, []);

    const stop = useCallback(async () => {
        await StopGridViewer();
        setRunning(false);
    }, []);

    return { running, error, logPath, start, stop };
}
