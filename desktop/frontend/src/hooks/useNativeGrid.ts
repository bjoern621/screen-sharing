import { useCallback, useEffect, useState } from "react";
import {
    NativeGridRunning,
    StartNativeGrid,
    StopNativeGrid,
} from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { PublishExit } from "../types/stream";

const POLL_INTERVAL_MS = 2000;

/**
 * Tracks the native grid window, a separate GTK binary the backend spawns.
 * running is polled, so a window closed via its own close button drops out;
 * "nativegrid:exit" carries the failure message.
 */
export function useNativeGrid() {
    const [running, setRunning] = useState(false);
    const [error, setError] = useState("");

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
                setError("native grid exited - " + e.message);
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

    const toggle = useCallback(async () => {
        setError("");
        try {
            if (running) {
                await StopNativeGrid();
                setRunning(false);
            } else {
                // RTSP is the one transport the relay re-serves every codec
                // on (MPEG-TS has no VP9 mapping, so SRT cannot carry it).
                await StartNativeGrid("rtsp");
                setRunning(true);
            }
        } catch (e) {
            setError("native grid: " + e);
        }
    }, [running]);

    return { running, error, toggle };
}
