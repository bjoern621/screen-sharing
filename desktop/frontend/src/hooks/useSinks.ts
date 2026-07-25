import { useCallback, useEffect, useRef, useState } from "react";
import { SinkKind, StreamSink } from "../types/sink";
import { createSink, SinkConfig } from "../services/sinks/createSink";

/**
 * Owns the live sinks for the grid, one per stream name. connect() builds a sink
 * for the chosen decoder and disconnect() tears it down. Sinks live in a ref
 * (created in connect(), not in an effect) so React StrictMode's double-invoke
 * cannot close a live connection; the roster mirrors the ref for rendering.
 *
 * Unmounting the grid closes every sink.
 */
export function useSinks(opts: SinkConfig) {
    const sinks = useRef(new Map<string, StreamSink>());
    const [roster, setRoster] = useState<Record<string, StreamSink>>({});

    const configRef = useRef<SinkConfig>(opts);
    configRef.current = opts;

    const connect = useCallback((name: string, kind: SinkKind) => {
        // Replace any existing sink for this name (a retry after failure).
        sinks.current.get(name)?.close();
        const sink = createSink(name, kind, configRef.current);
        sinks.current.set(name, sink);
        setRoster(prev => ({ ...prev, [name]: sink }));
    }, []);

    const disconnect = useCallback((name: string) => {
        sinks.current.get(name)?.close();
        sinks.current.delete(name);
        setRoster(prev => {
            const next = { ...prev };
            delete next[name];
            return next;
        });
    }, []);

    useEffect(() => {
        const map = sinks.current;
        return () => {
            for (const sink of map.values()) sink.close();
            map.clear();
        };
    }, []);

    return { sinks: roster, connect, disconnect };
}
