import { useCallback, useSyncExternalStore } from "react";
import { SinkSnapshot, StreamSink } from "../types/sink";

/** Subscription for a stream with no sink: nothing ever changes. */
const noSubscription = () => () => {};

/**
 * The live snapshot of a sink, or null for a stream that has none. Reading it
 * through useSyncExternalStore re-renders only the component watching that one
 * sink. The optional argument lets a roster entry watch a stream that is not
 * connected yet without a conditional hook.
 */
export function useSinkSnapshot(sink?: StreamSink): SinkSnapshot | null {
    const subscribe = useCallback(
        (cb: () => void) => (sink ? sink.subscribe(cb) : noSubscription()),
        [sink]
    );
    const getSnapshot = useCallback(() => sink?.getSnapshot() ?? null, [sink]);
    return useSyncExternalStore(subscribe, getSnapshot);
}

/**
 * Binds a tile to its sink: a ref callback that mounts the sink's render surface
 * into the tile container (and unmounts it on cleanup), plus the sink's live
 * snapshot. The callbacks are keyed on the sink identity, so the store
 * resubscribes only when the sink itself is swapped, not on every render.
 */
export function useSinkView(sink: StreamSink) {
    const snapshot = useSinkSnapshot(sink) as SinkSnapshot;

    const containerRef = useCallback(
        (el: HTMLElement | null) => {
            if (!el) return;
            sink.mount(el);
            return () => sink.unmount();
        },
        [sink]
    );

    return { containerRef, snapshot };
}
