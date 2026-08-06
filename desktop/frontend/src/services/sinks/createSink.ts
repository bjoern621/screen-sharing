import { SinkKind, StreamSink } from "../../types/sink";
import { MoqCertFetch, MoqSink } from "./MoqSink";
import { WebCodecsSink } from "./WebCodecsSink";
import { WhepSink } from "./WhepSink";

/** Connection facts a sink needs, supplied once by the owning hook. */
export interface SinkConfig {
    relayHost: string;
    /** WebRTC/WHEP listener port on the relay. */
    webrtcPort: number;
    /** Port of the in-app viewer service serving encoded frames over WebSocket. */
    viewerPort: number;
    /** Media-over-QUIC listener port on the relay: the WebTransport endpoint over
     * UDP and the fingerprint endpoint over TCP share the number. */
    moqPort: number;
    /**
     * Reads the relay's MoQ certificate fingerprint. Supplied as a callback rather
     * than called here because the fetch belongs to the backend - the app's origin
     * cannot verify a self-signed relay certificate - and this layer holds no
     * bindings (frontend-coding-style.md, "Layers").
     */
    moqCert: MoqCertFetch;
}

/** Builds the sink for one stream and decoder kind. */
export function createSink(
    name: string,
    kind: SinkKind,
    config: SinkConfig
): StreamSink {
    switch (kind) {
        case "whep":
            return new WhepSink(name, config.relayHost, config.webrtcPort);
        case "webcodecs":
            // The viewer service runs in-process (localhost); it pulls the stream
            // from the relay over RTSP and re-serves encoded frames here.
            return new WebCodecsSink(
                name,
                `ws://127.0.0.1:${config.viewerPort}/ws/${encodeURIComponent(name)}`
            );
        case "moq":
            // One hop, like WHEP: the page subscribes to the relay directly over
            // WebTransport, with no process on the receiver in between.
            return new MoqSink(
                name,
                `https://${config.relayHost}:${config.moqPort}/${encodeURIComponent(name)}/moq`,
                config.moqCert
            );
    }
}
