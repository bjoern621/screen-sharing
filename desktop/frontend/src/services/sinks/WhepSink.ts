import { SinkKind, SinkStats } from "../../types/sink";
import { bitrateMbps, ByteSample } from "../../util/bitrate";
import { HAS_WEBRTC } from "../../util/webgrid";
import { BaseSink } from "./BaseSink";
import { ElementAudioControl } from "./ElementAudioControl";

/**
 * Resolves once ICE gathering finishes, or after a short timeout so a stuck
 * gatherer cannot block the offer. Host candidates on a LAN arrive well within
 * the timeout.
 */
function iceComplete(pc: RTCPeerConnection): Promise<void> {
    if (pc.iceGatheringState === "complete") return Promise.resolve();
    return new Promise(resolve => {
        const timer = setTimeout(done, 1000);
        function done() {
            clearTimeout(timer);
            pc.removeEventListener("icegatheringstatechange", check);
            resolve();
        }
        function check() {
            if (pc.iceGatheringState === "complete") done();
        }
        pc.addEventListener("icegatheringstatechange", check);
    });
}

/**
 * A WHEP (RFC 9725) subscription rendered into a <video>. One
 * RTCPeerConnection: it POSTs a receive-only SDP offer to the relay's /whep
 * endpoint and collects the answered tracks into a MediaStream; close() DELETEs
 * the session resource and closes the connection.
 *
 * The relay re-serves every ingested stream over WHEP regardless of the publish
 * transport, but the browser only decodes H.264 video and Opus audio this way;
 * 4:4:4 codecs take the WebCodecs path instead.
 *
 * The peer connection is built inside openSession, not in the constructor, so a
 * webview whose WebRTC stack is missing or broken fails the sink with its error
 * message on the tile instead of throwing at whoever created the sink.
 */
export class WhepSink extends BaseSink {
    readonly kind: SinkKind = "whep";
    readonly audio: ElementAudioControl;

    private readonly video: HTMLVideoElement;
    private readonly stream: MediaStream;
    private pc?: RTCPeerConnection;
    private resource?: string;
    private closed = false;
    private lastBytes?: ByteSample;

    constructor(name: string, relayHost: string, webrtcPort: number) {
        super(name);
        this.video = document.createElement("video");
        this.video.autoplay = true;
        this.video.playsInline = true;
        this.video.className = "absolute inset-0 h-full w-full object-contain";
        this.stream = new MediaStream();
        this.video.srcObject = this.stream;
        this.audio = new ElementAudioControl(this.video, () => this.notify());

        // Decoded output, not a connected transport, is what counts as connected:
        // the tile holds its loading state until there is a picture behind it.
        // Either event is enough; setState ignores the second one.
        const rendered = () => {
            if (!this.closed) this.setState("connected");
        };
        this.video.addEventListener("loadeddata", rendered, { once: true });
        this.video.addEventListener("playing", rendered, { once: true });

        const endpoint = `http://${relayHost}:${webrtcPort}/${encodeURIComponent(name)}/whep`;
        void this.openSession(endpoint);
    }

    private async openSession(endpoint: string): Promise<void> {
        try {
            if (!HAS_WEBRTC) {
                throw new Error(
                    "this webview has no WebRTC support, so WHEP cannot play here; open the stream in a LAN browser or the native ffplay/mpv viewer",
                );
            }
            const pc = new RTCPeerConnection();
            this.pc = pc;
            if (this.closed) {
                pc.close();
                return;
            }
            pc.addTransceiver("video", { direction: "recvonly" });
            pc.addTransceiver("audio", { direction: "recvonly" });
            pc.ontrack = e => this.stream.addTrack(e.track);
            pc.onconnectionstatechange = () => {
                if (this.closed) return;
                const cs = pc.connectionState;
                if (cs === "connected") {
                    this.setPhase("buffering");
                } else if (cs === "failed" || cs === "disconnected") {
                    this.setState(
                        "failed",
                        cs === "failed"
                            ? "The WebRTC connection failed"
                            : "The WebRTC connection dropped"
                    );
                }
            };

            await pc.setLocalDescription(await pc.createOffer());
            await iceComplete(pc);
            if (this.closed) return;

            this.setPhase("negotiating");
            const res = await fetch(endpoint, {
                method: "POST",
                headers: { "Content-Type": "application/sdp" },
                body: pc.localDescription!.sdp,
            });
            if (this.closed) return;
            if (!res.ok) {
                throw new Error(`WHEP POST ${res.status} ${res.statusText}`);
            }
            const loc = res.headers.get("Location");
            if (loc) this.resource = new URL(loc, endpoint).toString();
            await pc.setRemoteDescription({
                type: "answer",
                sdp: await res.text(),
            });
        } catch (e) {
            if (!this.closed) this.setState("failed", String(e));
        }
    }

    mount(container: HTMLElement): void {
        if (this.video.parentElement !== container) {
            container.appendChild(this.video);
        }
        this.video.play().catch(() => {
            // Autoplay with sound is blocked without a user gesture; fall back to
            // muted playback and surface the auto-mute through the snapshot.
            this.audio.forceMuted();
            void this.video.play().catch(() => {});
        });
    }

    unmount(): void {
        this.video.remove();
    }

    async stats(): Promise<SinkStats | null> {
        if (this.closed || !this.pc) return null;
        const report = await this.pc.getStats();
        let video: RTCInboundRtpStreamStats | undefined;
        report.forEach(s => {
            if (s.type === "inbound-rtp") {
                const rtp = s as RTCInboundRtpStreamStats;
                if (rtp.kind === "video") video = rtp;
            }
        });
        if (!video) return null;

        let codec = "";
        if (video.codecId) {
            const c = report.get(video.codecId) as RTCRtpStreamStats & {
                mimeType?: string;
            };
            codec = c?.mimeType?.split("/")[1]?.toUpperCase() ?? "";
        }

        const sample: ByteSample = {
            bytes: video.bytesReceived ?? 0,
            timestamp: video.timestamp,
        };
        const mbps = this.lastBytes ? bitrateMbps(this.lastBytes, sample) : 0;
        this.lastBytes = sample;

        return {
            width: video.frameWidth ?? 0,
            height: video.frameHeight ?? 0,
            codec,
            transport: "webrtc",
            decoder: "WHEP",
            fps: video.framesPerSecond ?? 0,
            framesDecoded: video.framesDecoded ?? 0,
            framesDropped: video.framesDropped ?? 0,
            bitrateMbps: mbps,
            jitterMs: video.jitter !== undefined ? video.jitter * 1000 : undefined,
            packetsLost: video.packetsLost,
        };
    }

    close(): void {
        if (this.closed) return;
        this.closed = true;
        this.disarm();
        if (this.resource) {
            void fetch(this.resource, { method: "DELETE" }).catch(() => {});
        }
        this.pc?.close();
        for (const track of this.stream.getTracks()) track.stop();
    }
}
