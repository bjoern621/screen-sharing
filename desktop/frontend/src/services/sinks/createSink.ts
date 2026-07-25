import { SinkKind, StreamSink } from "../../types/sink";
import { WebCodecsSink } from "./WebCodecsSink";
import { WhepSink } from "./WhepSink";

/** Connection facts a sink needs, supplied once by the owning hook. */
export interface SinkConfig {
    relayHost: string;
    /** WebRTC/WHEP listener port on the relay. */
    webrtcPort: number;
    /** Port of the in-app viewer service serving encoded frames over WebSocket. */
    viewerPort: number;
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
    }
}
