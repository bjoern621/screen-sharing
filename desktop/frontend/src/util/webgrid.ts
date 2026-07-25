import { ViewVerdict, Stream } from "../types/stream";
import { SinkKind } from "../types/sink";
import {
    Capability,
    CHROMA_META,
    Chroma,
    Format,
    FORMAT_META,
    formatOf,
} from "./domain";

/**
 * One web-grid decode path. This is the single source both the web-grid
 * viewability verdict and the runtime decoder selection read, so the badge shown
 * in settings and the sink the grid actually builds can never disagree.
 *
 * whep decodes over WebRTC, where the webview takes H.264 4:2:0 only. webcodecs
 * decodes VP9 (4:4:4 included) from the viewer service over WebSocket. available
 * reports whether the host webview owns the API the path needs, so a build
 * without it produces a verdict instead of a runtime failure on the tile.
 *
 * Each path pins its own watch leg, relay to viewer, and neither follows the
 * publish leg: a stream published over SRT still reaches whep over WebRTC and
 * webcodecs over the viewer service's RTSP subscription.
 */
interface WebGridPath {
    decoder: Extract<SinkKind, "whep" | "webcodecs">;
    formats: Format[];
    /** whep needs 4:2:0; webcodecs takes any chroma its decoder supports. */
    requires420: boolean;
    available: boolean;
    /** How the verdict names this path, as a clause. */
    label: string;
}

/**
 * WebKitGTK carries the WebRTC bindings only when built with ENABLE_WEB_RTC,
 * which the nixpkgs default build leaves off, so the constructor is the
 * capability check (see viewer-architecture.md). Both paths are probed the same
 * way, since neither API is guaranteed by the webview the app is embedded in.
 */
export const HAS_WEBRTC = typeof RTCPeerConnection !== "undefined";
const HAS_WEBCODECS = typeof VideoDecoder !== "undefined";

export const WEB_GRID_DECODE: WebGridPath[] = [
    {
        decoder: "whep",
        formats: ["h264"],
        requires420: true,
        available: HAS_WEBRTC,
        label: "WHEP decodes H.264 4:2:0",
    },
    {
        decoder: "webcodecs",
        formats: ["vp9"],
        requires420: false,
        available: HAS_WEBCODECS,
        label: "the WebCodecs viewer decodes VP9 at any chroma",
    },
];

/**
 * Whether the web grid can decode the configured stream, with a reason either
 * way. Derived from the codec's format, the chroma's 4:2:0 flag and
 * WEB_GRID_DECODE, so it tracks the same tables the encoder and the sinks use.
 */
export function webGridCheck(s: Stream, caps: Capability[] | null): ViewVerdict {
    const fmt = formatOf(s.codec, caps);
    if (!fmt) {
        return { ok: false, text: "Checking web grid decode support…" };
    }
    const chroma = CHROMA_META[s.chroma as Chroma];
    const is420 = chroma?.is420 ?? false;
    for (const path of WEB_GRID_DECODE) {
        if (!path.available) continue;
        if (path.formats.includes(fmt) && (!path.requires420 || is420)) {
            return { ok: true, text: `Viewable in web grid - ${path.label}.` };
        }
    }

    // A format a live path already carries is blocked by its chroma, not by
    // itself: H.264 4:4:4 fails WHEP's 4:2:0 rule. Name whichever half is the gap,
    // and list the paths only when the format is the one missing.
    const fmtLabel = FORMAT_META[fmt]?.label ?? fmt;
    const carried = WEB_GRID_DECODE.some(
        p => p.available && p.formats.includes(fmt)
    );
    const paths = WEB_GRID_DECODE.filter(p => p.available).map(p => p.label);
    const gap =
        carried && chroma?.whepBlock
            ? `WHEP carries ${fmtLabel} but not ${chroma.whepBlock}`
            : `${fmtLabel} at ${s.chroma} has no web-grid path: ${
                  paths.length
                      ? paths.join(", and ")
                      : "this webview owns neither web-grid decoder"
              }`;
    const whep = HAS_WEBRTC
        ? ""
        : " This webview has no WebRTC, so H.264 plays over WHEP in a LAN browser but not here.";
    return {
        ok: false,
        text: `Not viewable in web grid - ${gap}. Watch it in the native grid.${whep}`,
    };
}

/** The video coding format named in a relay path's track list, if recognized. */
function formatFromTracks(tracks: string): Format | undefined {
    const t = tracks.toUpperCase();
    if (t.includes("VP9")) return "vp9";
    if (t.includes("VP8")) return "vp8";
    if (t.includes("AV1")) return "av1";
    if (t.includes("H265") || t.includes("H.265") || t.includes("HEVC")) return "hevc";
    if (t.includes("H264") || t.includes("H.264") || t.includes("AVC")) return "h264";
    return undefined;
}

/**
 * The decoder the web grid should use for a live relay path, chosen from its
 * track codecs against WEB_GRID_DECODE. Falls back to the first path the webview
 * owns when the codec is unknown or has no web-grid path, so the tile connects
 * and surfaces its own decode failure rather than silently doing nothing.
 */
export function sinkKindForTracks(tracks: string): SinkKind {
    const fmt = formatFromTracks(tracks);
    if (fmt) {
        for (const path of WEB_GRID_DECODE) {
            if (path.available && path.formats.includes(fmt)) return path.decoder;
        }
    }
    return WEB_GRID_DECODE.find(p => p.available)?.decoder ?? "whep";
}
