import { useEffect } from "react";
import { IconDownload, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
    Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { RelayStatus } from "../../types/stream";
import { WatchKey, watchId } from "../../hooks/useLive";
import { RTSP_PROTOCOLS } from "../../util/options";
import Tip from "../Tip/Tip";
import ErrorLog from "../ErrorLog/ErrorLog";
import NumberField from "../fields/NumberField";
import SelectField from "../fields/SelectField";

interface LiveNowCardProps {
    live: RelayStatus | null;
    watching: WatchKey[];
    /** Transports a stream can be received over, offered in the watch dropdown. */
    watchTransports: string[];
    /** Which of those can carry each bitstream format, keyed by format. */
    watchTransportsByFormat: Record<string, string[]>;
    /** Persisted transport selection for the watch dropdown. */
    watchTransport: string;
    connecting: Set<string>;
    error: string;
    logPath: string;
    /** Per-transport watch-leg knobs, each shown with the transport it belongs to. */
    srtWatchLatencyMs: number;
    rtspWatchLatencyMs: number;
    rtspWatchProtocol: string;
    onToggleWatch: (name: string, transport: string, isWatching: boolean) => void;
    onUpdateWatchTransport: (transport: string) => void;
    onUpdateSrtWatchLatency: (value: number) => void;
    onUpdateRtspWatchLatency: (value: number) => void;
    onUpdateRtspWatchProtocol: (value: string) => void;
    onOpenLog: (path: string) => void;
    onOpenLogsFolder: () => void;
}

/** Picks the initial watch transport: SRT when available, else the first
 * offered. Which streams that choice can then receive is a per-stream question,
 * answered by carryReason. */
function defaultTransport(watchTransports: string[]): string {
    if (watchTransports.includes("srt")) return "srt";
    return watchTransports[0] ?? "";
}

/** Why the selected transport cannot deliver this stream, empty when it can.
 *
 * The relay re-serves an ingested stream on the listeners whose protocol has a
 * payload mapping for its bitstream, and on no others: MPEG-TS carries H.264 and
 * H.265, so SRT cannot deliver a VP9 or AV1 stream however it was published. A
 * viewer opened on that pair connects and receives nothing, which reads as a
 * broken stream rather than an impossible combination.
 *
 * A stream whose format the snapshot does not name yet blocks nothing: the poll
 * can be older than the stream, and refusing on absent information would hide a
 * Watch control that would have worked. */
function carryReason(
    format: string,
    transport: string,
    byFormat: Record<string, string[]>
): string {
    const carried = byFormat[format];
    if (!format || !carried || carried.includes(transport)) return "";
    if (carried.length === 0) {
        return `${format} has no watch transport: no listener the relay serves carries it`;
    }
    return `${transport} cannot carry ${format}: watch it over ${carried.join(" or ")}`;
}

/** Relay reachability, the live-stream table with a per-row Watch/Stop control,
 * and the summed download bitrate of watched streams. The dropdown selects the
 * watch leg of the single-stream windows a Watch click opens, independent of the
 * transport a stream was published on, since the relay re-serves it on all its
 * listeners. The native grid keeps a leg of its own, set in its sidebar, because
 * a receiving pipeline and a player reach different protocol sets. The selected
 * transport's own knobs sit under the table and are the same settings fields the
 * grid reads, so a value set here reaches both viewers. */
export default function LiveNowCard({
    live,
    watching,
    watchTransports,
    watchTransportsByFormat,
    watchTransport,
    connecting,
    error,
    logPath,
    srtWatchLatencyMs,
    rtspWatchLatencyMs,
    rtspWatchProtocol,
    onToggleWatch,
    onUpdateWatchTransport,
    onUpdateSrtWatchLatency,
    onUpdateRtspWatchLatency,
    onUpdateRtspWatchProtocol,
    onOpenLog,
    onOpenLogsFolder,
}: LiveNowCardProps) {
    const paths = live?.paths ?? [];
    const downloadSum = paths
        .filter(p => watching.some(w => w.name === p.name))
        .reduce((a, p) => a + p.inMbps, 0);

    // The offered list only exists at runtime, so a persisted selection it no
    // longer contains is repaired here rather than in normalize.
    useEffect(() => {
        if (watchTransports.length === 0) return;
        if (!watchTransports.includes(watchTransport)) {
            onUpdateWatchTransport(defaultTransport(watchTransports));
        }
    }, [watchTransports, watchTransport, onUpdateWatchTransport]);

    const transport = watchTransports.includes(watchTransport)
        ? watchTransport
        : "";

    return (
        <Card>
            <CardHeader>
                <CardTitle className="text-base flex items-center gap-2">
                    Live now
                    {live ? (
                        live.reachable ? (
                            <Badge className="bg-green-500/10 text-green-600 dark:bg-green-500/20 dark:text-green-400">
                                relay available
                            </Badge>
                        ) : (
                            <Badge variant="destructive">
                                relay unreachable
                                {live.error ? `: ${live.error}` : ""}
                            </Badge>
                        )
                    ) : (
                        <Badge variant="secondary">
                            <IconLoader2 size={12} className="animate-spin" />{" "}
                            checking relay
                        </Badge>
                    )}
                </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
                <div className="flex items-center gap-2 text-sm">
                    <Tip text="Protocol a Watch click receives over, the watch leg (relay to viewer). The native grid runs on a leg of its own, picked in that window's sidebar, because a receiving pipeline reaches WHEP and a player opens the relay's HLS. It is independent of the publish transport in Stream settings and the two can differ, because the relay re-serves each ingested stream on its listeners. Which streams a given choice can deliver still follows their format: MPEG-TS over SRT carries H.264 and H.265, so a VP9 or AV1 row names the transport that carries it instead of offering Watch.">
                        <span className="text-muted-foreground">Watch over</span>
                    </Tip>
                    <Select
                        value={transport}
                        disabled={watchTransports.length === 0}
                        onValueChange={(v: string | null) =>
                            v && onUpdateWatchTransport(v)
                        }
                    >
                        <SelectTrigger className="w-[140px]">
                            <SelectValue>{(v: string) => v}</SelectValue>
                        </SelectTrigger>
                        <SelectContent>
                            {watchTransports.map(t => (
                                <SelectItem key={t} value={t}>
                                    {t}
                                </SelectItem>
                            ))}
                        </SelectContent>
                    </Select>
                </div>
                <Table>
                    <TableHeader>
                        <TableRow>
                            <TableHead>stream</TableHead>
                            <TableHead>codec</TableHead>
                            <TableHead>
                                <Tip text="Live ingest bitrate at the relay, derived from byte counters between 2 s polls.">
                                    <span>live bitrate</span>
                                </Tip>
                            </TableHead>
                            <TableHead>viewers</TableHead>
                            <TableHead />
                        </TableRow>
                    </TableHeader>
                    <TableBody>
                        {paths.map(p => {
                            const isWatching = watching.some(
                                w => w.name === p.name && w.transport === transport
                            );
                            const isConnecting = connecting.has(
                                watchId(p.name, transport)
                            );
                            const blocked = carryReason(
                                p.format,
                                transport,
                                watchTransportsByFormat
                            );
                            return (
                                <TableRow key={p.name}>
                                    <TableCell>
                                        {p.name}
                                        {p.ready ? "" : " (starting)"}
                                    </TableCell>
                                    <TableCell>{p.tracks}</TableCell>
                                    <TableCell>
                                        {p.inMbps.toFixed(1)} Mbps
                                    </TableCell>
                                    <TableCell>{p.readers}</TableCell>
                                    <TableCell>
                                        <Button
                                            size="sm"
                                            disabled={
                                                isConnecting ||
                                                !transport ||
                                                (!!blocked && !isWatching)
                                            }
                                            variant={
                                                isWatching
                                                    ? "outline"
                                                    : "default"
                                            }
                                            onClick={() =>
                                                onToggleWatch(
                                                    p.name,
                                                    transport,
                                                    isWatching
                                                )
                                            }
                                        >
                                            {isConnecting ? (
                                                <>
                                                    <IconLoader2
                                                        size={14}
                                                        className="animate-spin"
                                                    />{" "}
                                                    connecting
                                                </>
                                            ) : isWatching ? (
                                                "Stop"
                                            ) : (
                                                "Watch"
                                            )}
                                        </Button>
                                    </TableCell>
                                </TableRow>
                            );
                        })}
                    </TableBody>
                </Table>
                <div className="flex items-center gap-1 text-sm text-muted-foreground">
                    <IconDownload size={14} /> download (sum of watched):{" "}
                    {downloadSum.toFixed(1)} Mbit/s
                </div>
                {transport === "srt" && (
                    <div className="max-w-[230px]">
                        <NumberField
                            label="SRT watch latency (ms, watch leg)"
                            labelTip="SRT retransmit window for the watch leg (relay to viewer) - where internet loss usually lives. Applies to streams YOU watch over SRT; takes effect on the next Watch. SRT negotiates the larger of the two ends' windows, and MediaMTX asks for 120 ms, so anything below that is raised to it."
                            value={srtWatchLatencyMs}
                            min={1}
                            onChange={onUpdateSrtWatchLatency}
                        />
                    </div>
                )}
                {transport === "rtsp" && (
                    <div className="max-w-[320px] space-y-2">
                        <NumberField
                            label="RTSP jitter buffer (ms, watch leg)"
                            labelTip={"How long the native grid's receiver holds RTP packets before decoding, to reorder them and absorb network jitter. It is display delay, so keep it just above the link's jitter: 200 ms suits a LAN, a lossy remote link wants more. rtspsrc's own default is 2000 ms.\nffplay and mpv ignore it: they buffer by packet count, not by time."}
                            value={rtspWatchLatencyMs}
                            min={1}
                            onChange={onUpdateRtspWatchLatency}
                        />
                        <SelectField
                            label="RTSP transport (watch leg)"
                            labelTip="How RTP reaches the viewer inside the RTSP session. Every RTSP viewer reads it, single-stream windows and native grid tiles alike, when it opens: a running viewer keeps the transport it negotiated. The publish leg picks its own in stream settings."
                            value={rtspWatchProtocol}
                            options={RTSP_PROTOCOLS}
                            onChange={onUpdateRtspWatchProtocol}
                        />
                    </div>
                )}
                {error && (
                    <ErrorLog
                        message={error}
                        logPath={logPath}
                        onOpenLog={onOpenLog}
                        onOpenLogsFolder={onOpenLogsFolder}
                    />
                )}
            </CardContent>
        </Card>
    );
}
