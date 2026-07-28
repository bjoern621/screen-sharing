import { useCallback, useEffect, useState } from "react";
import {
    StartTestStreams,
    StopTestStreams,
    TestStreamsRunning,
} from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { PublishExit } from "../types/stream";

const POLL_INTERVAL_MS = 2000;

/**
 * Tracks the synthetic relay publishers (videotestsrc pushed over RTSP). The
 * relay serves them like real streams, so they exercise the grid and the
 * per-stream viewers without a screen capture running.
 * count is polled, so a publisher that died on its own drops out;
 * "teststream:exit" carries the failure message.
 */
export function useTestPublishers() {
    const [count, setCount] = useState(0);
    const [error, setError] = useState("");

    const refresh = useCallback(async () => {
        try {
            setCount(await TestStreamsRunning());
        } catch {
            /* backend not ready yet */
        }
    }, []);

    useEffect(() => {
        const off = EventsOn("teststream:exit", (e: PublishExit) => {
            if (e.message) {
                setError("Test stream exited: " + e.message);
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

    const start = useCallback(async (n: number) => {
        setError("");
        try {
            await StartTestStreams(n);
            setCount(n);
        } catch (e) {
            setError("Test streams: " + e);
        }
    }, []);

    const stop = useCallback(async () => {
        await StopTestStreams();
        setCount(0);
    }, []);

    return { count, error, start, stop };
}
