import { IconDownload, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { RelayStatus } from "../../types/stream";
import Tip from "../Tip/Tip";
import ErrorLog from "../ErrorLog/ErrorLog";
import NumberField from "../fields/NumberField";

interface LiveNowCardProps {
    live: RelayStatus | null;
    watching: string[];
    connecting: Set<string>;
    error: string;
    logPath: string;
    /** The viewer's configured transport; the SRT latency knob only exists for srt. */
    transport: string;
    watchLatencyMs: number;
    onToggleWatch: (name: string, isWatching: boolean) => void;
    onUpdateWatchLatency: (value: number) => void;
    onOpenLog: (path: string) => void;
    onOpenLogsFolder: () => void;
}

/** Relay reachability, the live-stream table with per-row Watch/Stop controls,
 * and the summed download bitrate of watched streams. */
export default function LiveNowCard({
    live,
    watching,
    connecting,
    error,
    logPath,
    transport,
    watchLatencyMs,
    onToggleWatch,
    onUpdateWatchLatency,
    onOpenLog,
    onOpenLogsFolder,
}: LiveNowCardProps) {
    const paths = live?.paths ?? [];
    const downloadSum = paths
        .filter(p => watching.includes(p.name))
        .reduce((a, p) => a + p.inMbps, 0);

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
                            const isWatching = watching.includes(p.name);
                            const isConnecting =
                                isWatching && connecting.has(p.name);
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
                                            disabled={isConnecting}
                                            variant={
                                                isWatching
                                                    ? "outline"
                                                    : "default"
                                            }
                                            onClick={() =>
                                                onToggleWatch(
                                                    p.name,
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
                            label="SRT watch latency (ms, hop 2)"
                            labelTip="SRT retransmit window for the viewer hop (relay to viewer) - where internet loss usually lives. Applies to streams YOU watch; takes effect on the next Watch."
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
