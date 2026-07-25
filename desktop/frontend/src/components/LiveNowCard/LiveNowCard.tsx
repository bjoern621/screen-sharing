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
import Tip from "../Tip/Tip";
import ErrorLog from "../ErrorLog/ErrorLog";
import NumberField from "../fields/NumberField";

interface LiveNowCardProps {
    live: RelayStatus | null;
    watching: WatchKey[];
    /** Transports a stream can be received over, offered in the watch dropdown. */
    watchTransports: string[];
    /** Persisted transport selection for the watch dropdown. */
    watchTransport: string;
    connecting: Set<string>;
    error: string;
    logPath: string;
    watchLatencyMs: number;
    onToggleWatch: (name: string, transport: string, isWatching: boolean) => void;
    onUpdateWatchTransport: (transport: string) => void;
    onUpdateWatchLatency: (value: number) => void;
    onOpenLog: (path: string) => void;
    onOpenLogsFolder: () => void;
}

/** Picks the initial watch transport: SRT when available, else the first
 * offered. The relay re-serves every stream on all its listeners, so any choice
 * receives any stream regardless of how it was published. */
function defaultTransport(watchTransports: string[]): string {
    if (watchTransports.includes("srt")) return "srt";
    return watchTransports[0] ?? "";
}

/** Relay reachability, the live-stream table with a per-row Watch/Stop control,
 * and the summed download bitrate of watched streams. A single dropdown selects
 * the transport every Watch click receives over, independent of the transport a
 * stream was published on, since the relay re-serves it on all its listeners.
 * The selection lives in the persisted settings, so it survives a restart. */
export default function LiveNowCard({
    live,
    watching,
    watchTransports,
    watchTransport,
    connecting,
    error,
    logPath,
    watchLatencyMs,
    onToggleWatch,
    onUpdateWatchTransport,
    onUpdateWatchLatency,
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
                    <Tip text="Protocol a Watch click receives over, the watch leg (relay to viewer). Any choice works for any stream: the relay re-serves each stream on all its listeners, so the watch leg is independent of the publish transport in Stream settings and the two can differ.">
                        <span className="text-muted-foreground">watch over</span>
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
                                            disabled={isConnecting || !transport}
                                            variant={
                                                isWatching ? "outline" : "default"
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
                            labelTip="SRT retransmit window for the watch leg (relay to viewer) - where internet loss usually lives. Applies to streams YOU watch over SRT; takes effect on the next Watch."
                            value={watchLatencyMs}
                            onChange={onUpdateWatchLatency}
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
