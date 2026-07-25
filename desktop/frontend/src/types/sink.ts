/**
 * The decode seam. A grid tile drives a stream through a StreamSink without
 * knowing how it is decoded: WHEP into a <video>, or (VP9 4:4:4) WebCodecs into
 * a <canvas>. The sink owns its render surface and its audio, so the tile stays
 * decoder-agnostic.
 */

/**
 * Connection lifecycle of one sink. "connected" means frames are on screen, not
 * merely that the transport came up, so a tile can hold its loading state until
 * there is something to see.
 */
export type SinkState = "connecting" | "connected" | "failed";

/**
 * The step a connecting sink is on, in order. Every decode path passes the same
 * three, so one tile UI narrates all of them: ask the relay for the stream,
 * agree on how to carry it, wait for the first frame to decode.
 */
export type SinkPhase = "requesting" | "negotiating" | "buffering";

/** The phases in progression order, for rendering a step indicator. */
export const SINK_PHASES: SinkPhase[] = ["requesting", "negotiating", "buffering"];

/** Which decoder backs a sink. Adding one is a new SinkKind + a new impl. */
export type SinkKind = "whep" | "webcodecs";

/** Mute and volume of a sink that has audio. */
export interface AudioSnapshot {
    muted: boolean;
    volume: number;
}

/**
 * Audio controls a sink may or may not expose. A video-only sink (the WebCodecs
 * canvas path carries no audio track) exposes null instead of these controls,
 * so a tile shows the volume UI only when audio exists.
 */
export interface AudioControl {
    setMuted(muted: boolean): void;
    setVolume(volume: number): void;
    getSnapshot(): AudioSnapshot;
}

/**
 * Immutable per-render view of a sink, read through useSyncExternalStore.
 * getSnapshot() must return the same reference until a real change, or the store
 * subscription loops.
 */
export interface SinkSnapshot {
    state: SinkState;
    /** How far the connection has come. Meaningful while state is "connecting". */
    phase: SinkPhase;
    error?: string;
    /** null when the sink has no audio. */
    audio: AudioSnapshot | null;
}

/**
 * Honest per-stream decode figures for the stats overlay. jitterMs and
 * packetsLost are WebRTC-only and absent on other decoders.
 */
export interface SinkStats {
    width: number;
    height: number;
    /** Negotiated codec, e.g. "H264", "VP9". */
    codec: string;
    /** Watch leg this tile receives over, relay to viewer, e.g. "webrtc",
     * "websocket", "local". Never the protocol the stream was published with. */
    transport: string;
    /** Decoder that rendered it, e.g. "WHEP", "WebCodecs". */
    decoder: string;
    fps: number;
    framesDecoded: number;
    framesDropped: number;
    bitrateMbps: number;
    jitterMs?: number;
    packetsLost?: number;
    /** Glass-to-glass latency in ms when the decoder can measure it. */
    latencyMs?: number;
}

/**
 * One decoded stream. The sink creates and owns its render element, manages its
 * connection, and reports state and stats. mount/unmount attach and detach the
 * surface; close() is the terminal teardown of the connection.
 */
export interface StreamSink {
    readonly name: string;
    readonly kind: SinkKind;
    /** Audio controls, or null when the sink carries no audio. */
    readonly audio: AudioControl | null;

    /** Register for state/audio changes; returns an unsubscribe. */
    subscribe(onChange: () => void): () => void;
    /** Current snapshot; stable reference until a real change. */
    getSnapshot(): SinkSnapshot;

    /**
     * Create (or reuse) the sink's <video>/<canvas>, append it to container,
     * fill it and start playback. Idempotent; safe to call again after unmount.
     */
    mount(container: HTMLElement): void;
    /** Detach the render surface. Does not tear down the connection. */
    unmount(): void;

    /** Latest decode figures, or null before any are available. */
    stats(): Promise<SinkStats | null>;
    /** Terminal teardown: close the connection and release resources. */
    close(): void;
}
