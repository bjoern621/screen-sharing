import { useCallback, useEffect, useState } from "react";
import { StartWall, StopWall, WallRunning } from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { PublishExit } from "../types/stream";

const POLL_INTERVAL_MS = 2000;

/**
 * Tracks the native grid window (the GStreamer compositor wall). start()
 * launches it over the given streams, stop() closes it. running is polled, so
 * a wall closed via its own window button is noticed too; "wall:exit" carries
 * the failure message when the pipeline died on its own.
 */
export function useWall() {
    const [running, setRunning] = useState(false);
    const [error, setError] = useState("");
    const [logPath, setLogPath] = useState("");

    const refresh = useCallback(async () => {
        try {
            setRunning(await WallRunning());
        } catch {
            /* backend not ready yet */
        }
    }, []);

    useEffect(() => {
        const off = EventsOn("wall:exit", (e: PublishExit) => {
            if (e.message) {
                setError("native grid exited - " + e.message);
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
            await StartWall(names, transport);
            setRunning(true);
        } catch (e) {
            setError("native grid: " + e);
        }
    }, []);

    const stop = useCallback(async () => {
        await StopWall();
        setRunning(false);
    }, []);

    return { running, error, logPath, start, stop };
}
