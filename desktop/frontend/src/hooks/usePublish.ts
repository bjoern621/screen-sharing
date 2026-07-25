import { useCallback, useEffect, useRef, useState } from "react";
import {
    StartPublish, StopPublish, Publishing,
} from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { PublishExit, Stats, Stream } from "../types/stream";

const ROLLING_WINDOW_MS = 5000;

/**
 * Manages the publish lifecycle and the derived insight figures: latest encoder
 * stats, the 5-second rolling mean bitrate and the session peak. Reads the
 * current settings so toggling publish always sends the live values.
 */
export function usePublish(s: Stream | null) {
    const [publishing, setPublishing] = useState(false);
    const [error, setError] = useState("");
    const [logPath, setLogPath] = useState("");
    const [stats, setStats] = useState<Stats | null>(null);
    const [avg5, setAvg5] = useState(0);
    const [peak, setPeak] = useState(0);
    const history = useRef<{ t: number; mbps: number }[]>([]);

    /** Drops the figures so a stopped session shows dashes instead of the last
     * values it happened to report. */
    const resetInsights = useCallback(() => {
        history.current = [];
        setStats(null);
        setAvg5(0);
        setPeak(0);
    }, []);

    useEffect(() => {
        void (async () => setPublishing(await Publishing()))();

        const offStats = EventsOn("publish:stats", (st: Stats) => {
            const now = Date.now();
            if (st.instMbps > 0) {
                history.current.push({ t: now, mbps: st.instMbps });
                while (
                    history.current.length &&
                    now - history.current[0].t > ROLLING_WINDOW_MS
                ) {
                    history.current.shift();
                }
                setPeak(prev => Math.max(prev, st.instMbps));
            }
            const h = history.current;
            setAvg5(h.length ? h.reduce((a, b) => a + b.mbps, 0) / h.length : 0);
            setStats(st);
        });

        const offExit = EventsOn("publish:exit", (e: PublishExit) => {
            setPublishing(false);
            resetInsights();
            setLogPath(e.logPath ?? "");
            if (e.message) {
                setError("publisher exited: " + e.message);
            }
        });

        return () => {
            offStats();
            offExit();
        };
    }, [resetInsights]);

    const toggle = useCallback(async () => {
        setError("");
        setLogPath("");
        try {
            if (publishing) {
                await StopPublish();
                setPublishing(false);
                resetInsights();
            } else if (s) {
                resetInsights();
                await StartPublish(s);
                setPublishing(true);
            }
        } catch (e) {
            setError("error: " + e);
        }
    }, [publishing, s, resetInsights]);

    return { publishing, error, logPath, stats, avg5, peak, toggle };
}
