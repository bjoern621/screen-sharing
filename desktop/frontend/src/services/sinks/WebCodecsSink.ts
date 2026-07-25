import { SinkKind, SinkStats } from "../../types/sink";
import { bitrateMbps, ByteSample } from "../../util/bitrate";
import { BaseSink } from "./BaseSink";

/**
 * WebSocket frame contract with the Go viewer service (webviewer package). Each
 * binary message is one encoded VP9 frame:
 *
 *   byte 0        flags: bit 0 = keyframe
 *   bytes 1..8    PTS in microseconds, unsigned 64-bit big-endian
 *   bytes 9..     the VP9 frame payload
 *
 * The header is HEADER_BYTES long. The frontend feeds each payload to a
 * WebCodecs VideoDecoder and draws the decoded VideoFrame to a canvas.
 */
const HEADER_BYTES = 9;
const KEYFRAME_FLAG = 0x01;

/** VP9 profile 1 (4:4:4), 8-bit. Verified against VideoDecoder.isConfigSupported. */
const VP9_CODEC = "vp09.01.10.08";

/**
 * Decodes VP9 4:4:4 in the page: a WebSocket delivers encoded frames from the
 * relay (re-served by the Go viewer service), a WebCodecs VideoDecoder decodes
 * them, and each VideoFrame is drawn to a <canvas>. This carries the lossless
 * 4:4:4 modes that WHEP cannot negotiate. Video only; audio is null.
 *
 * WebCodecs and VP9 profile 1 support is not universal (notably the WebKitGTK
 * window); an unsupported config fails the sink cleanly rather than rendering
 * nothing.
 */
export class WebCodecsSink extends BaseSink {
    readonly kind: SinkKind = "webcodecs";
    readonly audio = null;

    private canvas: HTMLCanvasElement;
    private ctx: CanvasRenderingContext2D;
    private ws?: WebSocket;
    private decoder?: VideoDecoder;
    private closed = false;

    private framesDecoded = 0;
    private lastWidth = 0;
    private lastHeight = 0;
    private bytes = 0;
    private lastBytes?: ByteSample;
    private lastFrames?: { count: number; timestamp: number };
    private lastLatencyMs?: number;

    constructor(
        readonly name: string,
        private wsUrl: string
    ) {
        super(name);
        this.canvas = document.createElement("canvas");
        this.canvas.className = "absolute inset-0 h-full w-full object-contain";
        this.ctx = this.canvas.getContext("2d")!;
        void this.start();
    }

    private async start(): Promise<void> {
        if (typeof VideoDecoder === "undefined") {
            this.setState("failed", "WebCodecs is not available in this browser");
            return;
        }
        try {
            const support = await VideoDecoder.isConfigSupported({
                codec: VP9_CODEC,
            });
            if (!support.supported) {
                this.setState("failed", `decoder rejects ${VP9_CODEC}`);
                return;
            }
        } catch (e) {
            this.setState("failed", String(e));
            return;
        }
        if (this.closed) return;

        this.decoder = new VideoDecoder({
            output: frame => this.draw(frame),
            error: e => {
                if (!this.closed) this.setState("failed", e.message);
            },
        });
        this.decoder.configure({ codec: VP9_CODEC });

        this.setPhase("negotiating");
        const ws = new WebSocket(this.wsUrl);
        ws.binaryType = "arraybuffer";
        this.ws = ws;
        // The socket carries frames the moment it opens; connected waits for one
        // to come out of the decoder, so the tile's loading state ends on a
        // picture rather than on a handshake.
        ws.onopen = () => this.setPhase("buffering");
        ws.onmessage = e => this.onFrame(e.data as ArrayBuffer);
        ws.onerror = () => {
            if (!this.closed) this.setState("failed", "websocket error");
        };
        ws.onclose = () => {
            if (!this.closed && this.state !== "failed") {
                this.setState("failed", "websocket closed");
            }
        };
    }

    private onFrame(buf: ArrayBuffer): void {
        if (this.closed || !this.decoder) return;
        if (buf.byteLength <= HEADER_BYTES) return;
        const view = new DataView(buf);
        const keyframe = (view.getUint8(0) & KEYFRAME_FLAG) !== 0;
        const ptsUs = Number(view.getBigUint64(1));
        const payload = new Uint8Array(buf, HEADER_BYTES);
        this.bytes += payload.byteLength;
        this.lastLatencyMs = undefined;

        this.decoder.decode(
            new EncodedVideoChunk({
                type: keyframe ? "key" : "delta",
                timestamp: ptsUs,
                data: payload,
            })
        );
    }

    private draw(frame: VideoFrame): void {
        if (this.closed) {
            frame.close();
            return;
        }
        if (
            this.canvas.width !== frame.displayWidth ||
            this.canvas.height !== frame.displayHeight
        ) {
            this.canvas.width = frame.displayWidth;
            this.canvas.height = frame.displayHeight;
        }
        this.ctx.drawImage(frame, 0, 0);
        this.lastWidth = frame.displayWidth;
        this.lastHeight = frame.displayHeight;
        this.framesDecoded++;
        frame.close();
        if (this.state === "connecting") this.setState("connected");
    }

    mount(container: HTMLElement): void {
        if (this.canvas.parentElement !== container) {
            container.appendChild(this.canvas);
        }
    }

    unmount(): void {
        this.canvas.remove();
    }

    async stats(): Promise<SinkStats | null> {
        if (this.lastWidth === 0) return null;
        const now = performance.now();
        const sample: ByteSample = { bytes: this.bytes, timestamp: now };
        const mbps = this.lastBytes ? bitrateMbps(this.lastBytes, sample) : 0;
        this.lastBytes = sample;

        let fps = 0;
        if (this.lastFrames) {
            const dt = (now - this.lastFrames.timestamp) / 1000;
            if (dt > 0) fps = (this.framesDecoded - this.lastFrames.count) / dt;
        }
        this.lastFrames = { count: this.framesDecoded, timestamp: now };

        return {
            width: this.lastWidth,
            height: this.lastHeight,
            codec: "VP9",
            transport: "websocket",
            decoder: "WebCodecs",
            fps,
            framesDecoded: this.framesDecoded,
            framesDropped: 0,
            bitrateMbps: mbps,
            latencyMs: this.lastLatencyMs,
        };
    }

    close(): void {
        if (this.closed) return;
        this.closed = true;
        this.disarm();
        this.ws?.close();
        if (this.decoder && this.decoder.state !== "closed") this.decoder.close();
    }
}
