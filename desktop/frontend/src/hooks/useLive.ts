import { useCallback, useEffect, useState } from "react";
import {
    Live, Watching, StartWatch, StopWatch,
} from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { RelayStatus, WatchExit } from "../types/stream";

const POLL_INTERVAL_MS = 2000;

/**
 * Polls the relay for the live-stream snapshot and tracks which streams this
 * client is watching, including the transient "connecting" state between
 * StartWatch and the relay reporting a reader on the path. The reader count is
 * the readiness signal: it does not depend on the viewer's window system, so it
 * works the same under X11 and Wayland.
 */
export function useLive() {
    const [live, setLive] = useState<RelayStatus | null>(null);
    const [watching, setWatching] = useState<string[]>([]);
    const [connecting, setConnecting] = useState<Set<string>>(new Set());
    const [error, setError] = useState("");
    const [logPath, setLogPath] = useState("");

    const refresh = useCallback(async () => {
        try {
            setLive(await Live());
            setWatching(await Watching());
        } catch {
            /* backend not ready yet */
        }
    }, []);

    const clearConnecting = useCallback((name: string) => {
        setConnecting(prev => {
            const next = new Set(prev);
            next.delete(name);
            return next;
        });
    }, []);

    useEffect(() => {
        const offExit = EventsOn("watch:exit", (e: WatchExit) => {
            clearConnecting(e.name);
            if (e.message) {
                setError(`viewer '${e.name}' exited - ${e.message}`);
                setLogPath(e.logPath ?? "");
            }
            void refresh();
        });
        return () => offExit();
    }, [clearConnecting, refresh]);

    // A watched path drops out of "connecting" once the relay shows a reader on
    // it. That reader is this client's viewer, and the signal arrives with the
    // regular Live() poll, so no window probe is needed.
    useEffect(() => {
        if (!live?.paths) return;
        setConnecting(prev => {
            if (prev.size === 0) return prev;
            const next = new Set(prev);
            for (const p of live.paths) {
                if (p.readers > 0) next.delete(p.name);
            }
            return next.size === prev.size ? prev : next;
        });
    }, [live]);

    useEffect(() => {
        void refresh();
        const id = setInterval(refresh, POLL_INTERVAL_MS);
        return () => clearInterval(id);
    }, [refresh]);

    const toggleWatch = useCallback(
        async (name: string, isWatching: boolean) => {
            setError("");
            setLogPath("");
            try {
                if (isWatching) {
                    await StopWatch(name);
                    clearConnecting(name);
                } else {
                    setConnecting(prev => new Set(prev).add(name));
                    await StartWatch(name);
                }
            } catch (e) {
                clearConnecting(name);
                setError("watch error: " + e);
            }
            void refresh();
        },
        [clearConnecting, refresh]
    );

    return { live, watching, connecting, error, logPath, toggleWatch };
}
