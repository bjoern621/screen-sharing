import { BrowserVerdict, Stream } from "../types/stream";
import { CHROMA_META, CODEC_META, Chroma, Codec } from "./domain";

/**
 * Determines whether the stream is playable in a plain web browser (via the
 * relay's HLS/WebRTC pages) and returns a reason either way. Browsers decode
 * 4:2:0 only, and HEVC only in Safari or with an OS extension. Both facts come
 * from the chroma and codec meta tables.
 */
export function browserCheck(s: Stream): BrowserVerdict {
    const chroma = CHROMA_META[s.chroma as Chroma];
    if (chroma && !chroma.is420) {
        return {
            ok: false,
            text: `Not viewable in browser - no browser decodes ${chroma.browserBlock ?? "this pixel format"}; browsers decode 4:2:0 only. App viewers unaffected.`,
        };
    }

    switch (CODEC_META[s.codec as Codec]?.browser) {
        case "universal":
            return {
                ok: true,
                text: `Viewable in any browser (H.264 4:2:0) - http://${s.relayHost}:8888/${s.name}/`,
            };
        case "modern":
            return {
                ok: true,
                text: `Viewable in modern browsers (AV1 4:2:0) - WebRTC page http://${s.relayHost}:8889/${s.name}/ is the reliable path.`,
            };
        case "safari-only":
            return {
                ok: false,
                text: "Mostly not viewable in browser - HEVC decodes only in Safari or with an OS HEVC extension. Pick H.264 + 4:2:0 for universal browser playback.",
            };
        default:
            return { ok: false, text: "Unknown codec/browser combination." };
    }
}
