import { useEffect, useState } from "react";
import { SinkStats, StreamSink } from "../types/sink";

/** How often the stats overlay refreshes. Bitrate deltas assume a steady poll. */
const STATS_POLL_MS = 1000;

/**
 * Polls a sink's decode stats while active (the overlay is open) and stops when
 * it closes, so the getStats/decoder queries cost nothing for a tile whose
 * overlay is hidden.
 */
export function useSinkStats(sink: StreamSink, active: boolean): SinkStats | null {
    const [stats, setStats] = useState<SinkStats | null>(null);

    useEffect(() => {
        if (!active) {
            setStats(null);
            return;
        }
        let alive = true;
        const poll = async () => {
            const s = await sink.stats();
            if (alive) setStats(s);
        };
        void poll();
        const id = setInterval(() => void poll(), STATS_POLL_MS);
        return () => {
            alive = false;
            clearInterval(id);
        };
    }, [sink, active]);

    return stats;
}
