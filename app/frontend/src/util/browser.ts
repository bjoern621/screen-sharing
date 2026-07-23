import { BrowserVerdict, Stream } from "../types/stream";

/**
 * Determines whether the stream is playable in a plain web browser (via the
 * relay's HLS/WebRTC pages) and returns a reason either way. Browsers decode
 * 4:2:0 only, and HEVC only in Safari or with an OS extension.
 */
export function browserCheck(s: Stream): BrowserVerdict {
    const is420 = s.chroma === "yuv420p" || s.chroma === "p010le";
    if (!is420) {
        const what =
            s.chroma === "gbrp" ? "RGB (HEVC Range Extensions)" : "4:4:4 chroma";
        return {
            ok: false,
            text: `Not viewable in browser - no browser decodes ${what}; browsers decode 4:2:0 only. App viewers unaffected.`,
        };
    }
    switch (s.codec) {
        case "h264_nvenc":
        case "libx264":
            return {
                ok: true,
                text: `Viewable in any browser (H.264 4:2:0) - http://${s.relayHost}:8888/${s.name}/`,
            };
        case "av1_nvenc":
            return {
                ok: true,
                text: `Viewable in modern browsers (AV1 4:2:0) - WebRTC page http://${s.relayHost}:8889/${s.name}/ is the reliable path.`,
            };
        case "hevc_nvenc":
            return {
                ok: false,
                text: "Mostly not viewable in browser - HEVC decodes only in Safari or with an OS HEVC extension. Pick H.264 + 4:2:0 for universal browser playback.",
            };
        default:
            return { ok: false, text: "Unknown codec/browser combination." };
    }
}
