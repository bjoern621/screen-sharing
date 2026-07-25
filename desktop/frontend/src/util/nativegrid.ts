import { ViewVerdict, Stream } from "../types/stream";
import { Capability, findCapability, Format, FORMAT_META } from "./domain";

/**
 * The transport the native grid subscribes over, matching the one `useNativeGrid`
 * launches the window with. RTSP is the only listener the relay re-serves every
 * codec on, since MPEG-TS (SRT) has no VP9 mapping.
 */
export const NATIVE_GRID_TRANSPORT = "rtsp";

/**
 * Whether the native grid can decode the configured stream, with a reason either
 * way. A tile decodes through `decodebin`, so its breadth is libavcodec's: every
 * format the app can encode, at any chroma and bit depth. That leaves the
 * transport as the only gate, and the backend capability table already carries
 * which transports a codec has, so this verdict reads that table rather than
 * restating it.
 */
export function nativeGridCheck(
    s: Stream,
    caps: Capability[] | null
): ViewVerdict {
    const cap = findCapability(caps, s.codec);
    if (!cap) {
        return { ok: false, text: "Checking native grid decode support…" };
    }

    const fmtLabel = FORMAT_META[cap.format as Format]?.label ?? cap.format;
    if (!cap.transports.includes(NATIVE_GRID_TRANSPORT)) {
        return {
            ok: false,
            text: `Not viewable in native grid - no ${NATIVE_GRID_TRANSPORT.toUpperCase()} listener carries ${fmtLabel}, and that is the transport the grid subscribes over.`,
        };
    }
    return {
        ok: true,
        text: `Viewable in native grid - libavcodec decodes ${fmtLabel} at ${s.chroma} over ${NATIVE_GRID_TRANSPORT.toUpperCase()}, with no chroma or bit-depth limit.`,
    };
}
