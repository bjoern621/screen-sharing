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
 * Each path decodes one profile rather than a codec at large, so it fixes a
 * subsampling and a bit depth together: whep negotiates the 8-bit 4:2:0 H.264
 * profiles over WebRTC, and webcodecs declares VP9 profile 1, 8-bit full chroma,
 * to the VideoDecoder it feeds from the viewer service. available reports whether
 * the host webview owns the API the path needs, so a build without it produces a
 * verdict instead of a runtime failure on the tile.
 *
 * Each path pins its own watch leg, relay to viewer, and none follows the publish
 * leg: a stream published over SRT still reaches whep over WebRTC, webcodecs over
 * the viewer service's RTSP subscription, and moq over WebTransport.
 */
interface WebGridPath {
    decoder: SinkKind;
    formats: Format[];
    /** Subsampling of the profile the path decodes, matched exactly in both
     * directions: a 4:2:0 profile refuses full chroma and a full-chroma one
     * refuses 4:2:0. */
    is420: boolean;
    /** Bits per component the profile codes. */
    bitDepth: number;
    /** The codec string the path declares to `VideoDecoder`, where it decodes in
     * the page. whep negotiates its profile with the relay over SDP and declares
     * none, so the field is absent there. */
    codecString?: string;
    available: boolean;
    /** How the verdict names this path, as a clause. */
    label: string;
}

/**
 * WebKitGTK carries the WebRTC bindings only when built with ENABLE_WEB_RTC,
 * which the nixpkgs default build leaves off, so the constructor is the
 * capability check (see viewer-architecture.md). Every path is probed the same
 * way, since none of these APIs is guaranteed by the webview the app is embedded
 * in: WebTransport in particular is a Chromium-family binding, so the MoQ rows
 * drop out of the WebKitGTK window and stay in a LAN browser.
 */
export const HAS_WEBRTC = typeof RTCPeerConnection !== "undefined";
const HAS_WEBCODECS = typeof VideoDecoder !== "undefined";
export const HAS_WEBTRANSPORT = typeof WebTransport !== "undefined";

/**
 * The formats the relay re-serves over Media over QUIC. Unlike the other two
 * paths, MoQ pins no profile of its own: the catalog names the codec and the
 * reader hands that string to a VideoDecoder, so the limit is the webview's
 * WebCodecs reach rather than the leg's.
 *
 * The rows below therefore claim the one profile every WebCodecs implementation
 * accepts, 8-bit 4:2:0, and claim it for all five formats. A full-chroma stream
 * may well decode here on a Chromium webview, but the verdict does not promise
 * what only some hosts deliver: it under-promises and the tile over-delivers,
 * which is the right way round for a badge shown before anyone is watching.
 */
const MOQ_FORMATS: Format[] = ["h264", "hevc", "av1", "vp9", "vp8"];

export const WEB_GRID_DECODE: WebGridPath[] = [
    {
        decoder: "whep",
        formats: ["h264"],
        is420: true,
        bitDepth: 8,
        available: HAS_WEBRTC,
        label: "WHEP decodes 8-bit 4:2:0 H.264",
    },
    {
        decoder: "webcodecs",
        formats: ["vp9"],
        is420: false,
        bitDepth: 8,
        codecString: "vp09.01.10.08",
        available: HAS_WEBCODECS,
        label: "the WebCodecs viewer decodes VP9 profile 1, 8-bit full chroma",
    },
    // Last, so a format an earlier path already carries keeps the path it had:
    // H.264 4:2:0 stays on WHEP, which is one hop and needs no certificate, and
    // VP9 full chroma stays on the viewer service. What MoQ adds is the formats
    // neither of them reaches at all.
    {
        decoder: "moq",
        formats: MOQ_FORMATS,
        is420: true,
        bitDepth: 8,
        available: HAS_WEBTRANSPORT && HAS_WEBCODECS,
        label: "Media over QUIC decodes 8-bit 4:2:0 H.264, HEVC, AV1, VP9 and VP8",
    },
];

/**
 * The codec string a decode path declares to `VideoDecoder`, or undefined where
 * the path negotiates its profile instead of declaring one. `WebCodecsSink` reads
 * its configuration from here, so the profile the verdict promises is the profile
 * the decoder is given.
 */
export function webGridCodecString(decoder: SinkKind): string | undefined {
    return WEB_GRID_DECODE.find(p => p.decoder === decoder)?.codecString;
}

/**
 * Whether the web grid can decode the configured stream, with a reason either
 * way. Derived from the codec's format and the chroma's subsampling and bit depth
 * against WEB_GRID_DECODE, so it tracks the same tables the encoder and the sinks
 * use. A pixel format outside CHROMA_META matches no path and reports
 * not-viewable, rather than being read as one of the two the paths accept.
 */
export function webGridCheck(s: Stream, caps: Capability[] | null): ViewVerdict {
    const fmt = formatOf(s.codec, caps);
    if (!fmt) {
        return { ok: false, text: "Checking web grid decode support…" };
    }
    const chroma = CHROMA_META[s.chroma as Chroma];
    for (const path of WEB_GRID_DECODE) {
        if (!path.available) continue;
        if (
            path.formats.includes(fmt) &&
            path.is420 === chroma?.is420 &&
            path.bitDepth === chroma?.bitDepth
        ) {
            return { ok: true, text: `Viewable in web grid - ${path.label}.` };
        }
    }

    // whepBlock names what a pixel format asks beyond the 8-bit 4:2:0 profiles
    // WHEP negotiates, so it states the gap only where WHEP is the path carrying
    // the format. Otherwise the live paths are listed with the profiles they do
    // decode, which is what the combination missed.
    const fmtLabel = FORMAT_META[fmt]?.label ?? fmt;
    const whepCarries = WEB_GRID_DECODE.some(
        p => p.decoder === "whep" && p.available && p.formats.includes(fmt)
    );
    const paths = WEB_GRID_DECODE.filter(p => p.available).map(p => p.label);
    const gap =
        whepCarries && chroma?.whepBlock
            ? `WHEP carries ${fmtLabel} but not ${chroma.whepBlock}`
            : `${fmtLabel} at ${s.chroma} has no web-grid path: ${
                  paths.length
                      ? paths.join(", and ")
                      : "this webview owns no web-grid decoder"
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
 * track codecs against WEB_GRID_DECODE. A track list names the format and not the
 * profile, so the match is by format alone and a profile the path cannot decode
 * fails on the tile. Falls back to the first path the webview owns when the codec
 * is unknown or has no web-grid path, so the tile connects and surfaces its own
 * decode failure rather than silently doing nothing.
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
