/**
 * Types for the vendored MediaMTX Media-over-QUIC reader. The implementation is
 * `mediamtxMoqReader.js`, kept as upstream JavaScript so a re-vendor stays a copy
 * plus the five marked patches; this file is the contract MoqSink codes against.
 */

/** Decode figures the reader keeps, cumulative. Rates are the caller's to derive. */
export interface MoqReaderStats {
    /** The catalog's codec string, e.g. "avc3.64001f", "hev1.1.6.L120.90". */
    codec: string;
    width: number;
    height: number;
    framesDecoded: number;
    bytesReceived: number;
    hasAudio: boolean;
}

export interface MoqReaderConf {
    /**
     * SHA-256 of the relay's certificate, hex-encoded, pinned through
     * WebTransport's serverCertificateHashes. Supplied by the app (fetched in Go);
     * when absent the reader fetches it from fingerprintUrl itself, which only
     * works from an origin that already trusts the certificate.
     */
    fingerprint?: string;
    /** Where the reader fetches the fingerprint when none was supplied. */
    fingerprintUrl?: string;
    /** The WebTransport endpoint, https://host:port/<stream>/moq. */
    url: string;
    user?: string;
    pass?: string;
    token?: string;
    /** Container the reader appends its own canvas to. */
    videoElement: HTMLElement;
    onError?: (err: string) => void;
    onSubscribed?: (hasAudio: boolean) => void;
    onAudioMuted?: (muted: boolean) => void;
    /** Called on every drawn frame. */
    onFrame?: () => void;
}

/**
 * A Media-over-QUIC subscription decoding into a canvas it owns. It reconnects on
 * its own after a failure, reporting each one through onError, until close().
 */
export class MediaMTXMoQReader {
    constructor(conf: MoqReaderConf);
    /** Resume the AudioContext, which starts suspended without a user gesture. */
    unmute(): void;
    /** Suspend or resume the AudioContext. Muting is the same suspension. */
    setMuted(muted: boolean): void;
    /** Playback volume, 0..1. Held across the reader's own reconnects. */
    setVolume(volume: number): void;
    /** Terminal teardown; idempotent. Stops the reconnect loop. */
    close(): void;
    /** Decode figures, or null before a frame has been drawn. */
    stats(): MoqReaderStats | null;
}
