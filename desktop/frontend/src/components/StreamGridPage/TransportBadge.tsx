import { Badge } from "@/components/ui/badge";
import { SinkKind } from "../../types/sink";

/** The watch leg each decoder reaches the stream over, shown on the tile. The
 * names are the ones the sink reports as its stats transport and the app's
 * settings offer, so the badge and the stats overlay never spell a transport two
 * ways. A tile never shows the publish leg, which the viewer side cannot observe. */
const TRANSPORT_LABEL: Record<SinkKind, string> = {
    whep: "webrtc",
    webcodecs: "websocket",
};

interface TransportBadgeProps {
    kind: SinkKind;
    /** Negotiated codec, once known from the sink's stats. */
    codec?: string;
}

/** Per-tile badge naming the watch leg in use and the negotiated codec, e.g.
 * "webrtc · H264". Replaces the former page-level WHEP badge, which was wrong
 * once the grid could decode over more than one path. */
export default function TransportBadge({ kind, codec }: TransportBadgeProps) {
    return (
        <Badge
            variant="outline"
            className="border-white/20 bg-black/60 text-white backdrop-blur-sm"
        >
            {TRANSPORT_LABEL[kind]}
            {codec ? ` · ${codec}` : ""}
        </Badge>
    );
}
