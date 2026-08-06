import { SinkKind, SinkStats } from "../../types/sink";
import { bitrateMbps, ByteSample } from "../../util/bitrate";
import { BaseSink } from "./BaseSink";
import { MoqAudioControl } from "./MoqAudioControl";
import { MediaMTXMoQReader } from "./vendor/mediamtxMoqReader";

/** The relay's Media-over-QUIC certificate, as the sink needs it. */
export interface MoqCert {
    /** SHA-256 of the certificate, hex-encoded. */
    fingerprint: string;
    /** Whether it chained to a trusted root, or was taken on trust and pinned. */
    verified: boolean;
}

/** Fetches the relay's MoQ certificate for one stream. */
export type MoqCertFetch = (streamName: string) => Promise<MoqCert>;

/**
 * A Media-over-QUIC subscription decoded in the page, rendered into a canvas the
 * vendored reader owns.
 *
 * It is the widest of the web grid's decode paths and the only one carrying all
 * five formats, because the relay's catalog names the codec and the reader hands
 * that string to a VideoDecoder rather than pinning one profile the way the WHEP
 * and viewer-service paths do. What it will actually decode is therefore the host
 * webview's WebCodecs reach, not a property of this class.
 *
 * It is a watch leg with no publish counterpart: nothing this app drives publishes
 * MoQ, so a MoQ tile is always watching a stream the relay ingested over some
 * other protocol and re-serves here (viewer-architecture.md, "Reading Media over
 * QUIC").
 *
 * The certificate is settled before the subscription, and by the app rather than
 * by the page: WebTransport refuses a plain listener, and the relay's certificate
 * is self-signed by default, which the app's own origin has never accepted. The
 * fingerprint fetched through certFetch is pinned via serverCertificateHashes.
 *
 * The reader reconnects on its own after a failure. That is left in place - a
 * dropped stream coming back on its own is what a grid tile wants - so a reported
 * error fails the sink only once the connect deadline has passed, and until then
 * it is narration.
 */
export class MoqSink extends BaseSink {
    readonly kind: SinkKind = "moq";
    readonly audio: MoqAudioControl;

    private readonly surface: HTMLDivElement;
    private reader?: MediaMTXMoQReader;
    private closed = false;
    private verified = false;

    private lastBytes?: ByteSample;
    private lastFrames?: { count: number; timestamp: number };

    constructor(
        readonly name: string,
        private url: string,
        certFetch: MoqCertFetch
    ) {
        super(name);
        this.surface = document.createElement("div");
        this.surface.className = "absolute inset-0 h-full w-full";
        this.audio = new MoqAudioControl(
            () => this.reader,
            () => this.notify()
        );
        void this.start(certFetch);
    }

    private async start(certFetch: MoqCertFetch): Promise<void> {
        if (typeof WebTransport === "undefined") {
            this.setState(
                "failed",
                "this webview has no WebTransport, so Media over QUIC cannot play here; open the stream in a LAN browser or the native grid"
            );
            return;
        }

        let cert: MoqCert;
        try {
            cert = await certFetch(this.name);
        } catch (e) {
            this.setState("failed", `The relay's MoQ certificate could not be read: ${e}`);
            return;
        }
        if (this.closed) return;
        this.verified = cert.verified;

        this.setPhase("negotiating");
        const reader = new MediaMTXMoQReader({
            url: this.url,
            fingerprint: cert.fingerprint,
            videoElement: this.surface,
            // The reader retries by itself, which splits the error case in two.
            // Before the first frame it is narration: the connect deadline in
            // BaseSink is already running and is what ends a wait that will not
            // resolve, so failing here would only pre-empt it with a worse
            // message. After the first frame it is a drop the viewer should see.
            onError: err => {
                if (this.closed) return;
                if (this.state === "connecting") {
                    this.setPhase("negotiating");
                } else {
                    this.setState("failed", err);
                }
            },
            onSubscribed: () => {
                if (!this.closed) this.setPhase("buffering");
            },
            // Unconditional, because a reader that reconnected on its own is the
            // other half of the split above: the tile went to failed on the drop
            // and has to come back when frames resume, without a retry click.
            onFrame: () => {
                if (!this.closed) this.setState("connected");
            },
            onAudioMuted: muted => {
                if (!this.closed) this.audio.reportMuted(muted);
            },
        });
        if (this.closed) {
            reader.close();
            return;
        }
        this.reader = reader;
        this.audio.applyTo(reader);
    }

    mount(container: HTMLElement): void {
        if (this.surface.parentElement !== container) {
            container.appendChild(this.surface);
        }
    }

    unmount(): void {
        this.surface.remove();
    }

    async stats(): Promise<SinkStats | null> {
        const s = this.reader?.stats();
        if (!s) return null;

        const now = performance.now();
        const sample: ByteSample = { bytes: s.bytesReceived, timestamp: now };
        const mbps = this.lastBytes ? bitrateMbps(this.lastBytes, sample) : 0;
        this.lastBytes = sample;

        let fps = 0;
        if (this.lastFrames) {
            const dt = (now - this.lastFrames.timestamp) / 1000;
            if (dt > 0) fps = (s.framesDecoded - this.lastFrames.count) / dt;
        }
        this.lastFrames = { count: s.framesDecoded, timestamp: now };

        return {
            width: s.width,
            height: s.height,
            codec: codecLabel(s.codec),
            // One spelling of the leg, matching the tile badge; whether the relay
            // proved its identity is its own row rather than a second transport
            // name (TransportBadge, "never spell a transport two ways").
            transport: "moq",
            certPinned: !this.verified,
            decoder: "MoQ",
            fps,
            framesDecoded: s.framesDecoded,
            // WebCodecs exposes no dropped-frame counter, as on the viewer-service
            // path. NaN says the figure was never taken, where a zero would read
            // as a measurement. The reader does skip frames when its decode queue
            // is full, so a zero here would be wrong as well as unmeasured.
            framesDropped: NaN,
            bitrateMbps: mbps,
        };
    }

    close(): void {
        if (this.closed) return;
        this.closed = true;
        this.disarm();
        this.reader?.close();
    }
}

/**
 * The format name behind a catalog codec string. The overlay names formats the
 * way the rest of the app does, and the catalog spells them as MP4 sample-entry
 * codes with a profile suffix ("avc3.64001f", "hev1.1.6.L120.90").
 */
function codecLabel(codec: string): string {
    const prefix = codec.split(".")[0];
    switch (prefix) {
        case "avc1":
        case "avc3":
            return "H264";
        case "hev1":
        case "hvc1":
            return "H265";
        case "av01":
            return "AV1";
        case "vp09":
            return "VP9";
        case "vp8":
            return "VP8";
        default:
            // A catalog naming something outside the relay's own list is worth
            // showing verbatim rather than blanking: the overlay is where an
            // unexpected stream is diagnosed.
            return codec.toUpperCase();
    }
}
