import {
    AudioControl,
    SinkKind,
    SinkPhase,
    SinkSnapshot,
    SinkState,
    SinkStats,
    StreamSink,
} from "../../types/sink";

/**
 * How long a sink may stay in "connecting" before it reports failure. A stream
 * that never produces a frame looks exactly like a slow one, so the wait ends
 * either way and the tile offers a retry instead of spinning forever.
 */
const CONNECT_TIMEOUT_MS = 15000;

/**
 * Shared state machinery for every sink: the subscriber set, the cached
 * snapshot, the state transition helper and the connect deadline. Subclasses add
 * their render surface (a <video> or a <canvas>) and their connection.
 *
 * The snapshot is computed lazily and re-cached only on a real change, so
 * getSnapshot() returns a stable reference between changes as
 * useSyncExternalStore requires.
 */
export abstract class BaseSink implements StreamSink {
    abstract readonly kind: SinkKind;
    abstract readonly audio: AudioControl | null;

    protected state: SinkState = "connecting";
    protected phase: SinkPhase = "requesting";
    protected error?: string;

    private subscribers = new Set<() => void>();
    private snapshot: SinkSnapshot | null = null;
    private deadline: number;

    constructor(readonly name: string) {
        this.deadline = window.setTimeout(() => {
            this.setState(
                "failed",
                `no video after ${CONNECT_TIMEOUT_MS / 1000} s - the stream may have stopped, or its codec may not decode here`
            );
        }, CONNECT_TIMEOUT_MS);
    }

    subscribe(onChange: () => void): () => void {
        this.subscribers.add(onChange);
        return () => {
            this.subscribers.delete(onChange);
        };
    }

    getSnapshot(): SinkSnapshot {
        return (this.snapshot ??= this.compute());
    }

    /** Recompute the cached snapshot and wake subscribers. */
    protected notify(): void {
        this.snapshot = this.compute();
        for (const cb of this.subscribers) cb();
    }

    private compute(): SinkSnapshot {
        return {
            state: this.state,
            phase: this.phase,
            error: this.error,
            audio: this.audio?.getSnapshot() ?? null,
        };
    }

    protected setState(state: SinkState, error?: string): void {
        if (state === this.state && error === this.error) return;
        this.state = state;
        this.error = error;
        if (state !== "connecting") this.disarm();
        this.notify();
    }

    /** Advance the connecting narration. Ignored once connecting is over. */
    protected setPhase(phase: SinkPhase): void {
        if (this.state !== "connecting" || phase === this.phase) return;
        this.phase = phase;
        this.notify();
    }

    /** Drop the connect deadline. close() calls this so a torn-down sink cannot
     * report a timeout. */
    protected disarm(): void {
        clearTimeout(this.deadline);
    }

    abstract mount(container: HTMLElement): void;
    abstract unmount(): void;
    abstract stats(): Promise<SinkStats | null>;
    abstract close(): void;
}
