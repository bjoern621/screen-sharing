import { ViewVerdict, Stream } from "../types/stream";
import { Capability, findCapability, Format, FORMAT_META } from "./domain";

/**
 * The watch leg the native grid subscribes over, relay to viewer, matching the
 * transport `useNativeGrid` launches the window with. Fixed, and unrelated to the
 * publish leg: the relay re-serves each ingested stream on all its listeners, and
 * RTSP is the only listener carrying every codec the app encodes, since MPEG-TS
 * (SRT) has no VP9 mapping.
 */
export const NATIVE_GRID_TRANSPORT = "rtsp";

/**
 * Whether the native grid can decode the configured stream, with a reason either
 * way. A tile decodes through `decodebin`, which autoplugs whichever installed
 * decoder accepts the stream's caps: a hardware element where it advertises the
 * profile, a software one otherwise. Between them they cover every format the app
 * can encode, at any chroma and bit depth. That leaves the watch leg as the only
 * gate. The capability table lists publish transports, but a codec carries over
 * RTSP for publish exactly when RTP has a payload mapping for it, which is what
 * the relay needs to re-serve it as well, so the same column answers the watch
 * question and no second table restates it.
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
            text: `Not viewable in native grid - no ${NATIVE_GRID_TRANSPORT.toUpperCase()} listener carries ${fmtLabel}, and ${NATIVE_GRID_TRANSPORT.toUpperCase()} is how the grid receives from the relay, whatever protocol the stream was published over.`,
        };
    }
    return {
        ok: true,
        text: `Viewable in native grid - the grid receives ${fmtLabel} at ${s.chroma} from the relay over ${NATIVE_GRID_TRANSPORT.toUpperCase()}, independent of the publish transport: decodebin picks a hardware decoder where the profile fits and a software one otherwise, so no chroma or bit depth is excluded.`,
    };
}
