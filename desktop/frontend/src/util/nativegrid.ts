import { ViewVerdict, Stream, TransportCarriage } from "../types/stream";
import {
    Capability, carriersOf, carriesFormat, findCapability, Format, FORMAT_META,
} from "./domain";

/**
 * Whether the native grid can decode the configured stream, with a reason either
 * way. A tile decodes through `decodebin`, which autoplugs whichever installed
 * decoder accepts the stream's caps: a hardware element where it advertises the
 * profile, a software one otherwise. Between them they cover every format the app
 * can encode, at any chroma and bit depth. That leaves the watch leg as the only
 * gate, and the grid opens on the same leg as every other viewer this app starts
 * (`watchTransport`, the "Watch over" dropdown), so the verdict follows the
 * selection.
 *
 * The verdict reads the watch half of the transport table, not the publish half:
 * the relay serves a protocol formats it never ingests over that same protocol,
 * and refuses others it does ingest, so the leg a viewer receives on has a set of
 * its own.
 */
export function nativeGridCheck(
    s: Stream,
    caps: Capability[] | null,
    carriage: TransportCarriage[] | null
): ViewVerdict {
    const cap = findCapability(caps, s.codec);
    if (!cap || !carriage) {
        return { ok: false, text: "Checking native grid decode support…" };
    }

    const transport = s.watchTransport.toUpperCase();
    const fmtLabel = FORMAT_META[cap.format as Format]?.label ?? cap.format;
    if (!carriesFormat(carriage, s.watchTransport, "watch", cap.format)) {
        const carriers = carriersOf(carriage, "watch", cap.format);
        const alternatives = carriers.length
            ? ` Switch "Watch over" to ${carriers.join(" or ")} for it.`
            : "";
        return {
            ok: false,
            text: `Not viewable in native grid - the relay serves no ${fmtLabel} over ${transport}, and ${transport} is the watch leg the grid opens on, whatever protocol the stream was published over.${alternatives}`,
        };
    }
    return {
        ok: true,
        text: `Viewable in native grid - the grid receives ${fmtLabel} at ${s.chroma} from the relay over ${transport}, independent of the publish transport: decodebin picks a hardware decoder where the profile fits and a software one otherwise, so no chroma or bit depth is excluded.`,
    };
}
