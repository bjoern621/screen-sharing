import { ReactNode } from "react";
import { IconLoader2 } from "@tabler/icons-react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
    Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Stats } from "../../types/stream";
import { dropPercent, mbps } from "../../util/format";
import Tip from "../Tip/Tip";

interface PublishInsightsCardProps {
    stats: Stats | null;
    avg5: number;
    peak: number;
    targetFps: number;
    uplinkMbps: number;
    publishing: boolean;
}

/** Header cell with an explanatory tooltip. */
function HeadTip({ text, children }: { text: string; children: ReactNode }) {
    return (
        <TableHead>
            <Tip text={text}>
                <span>{children}</span>
            </Tip>
        </TableHead>
    );
}

/** Live encoder insights: instantaneous vs rolling vs cumulative bitrate, fps,
 * drops, speed and the uplink comparison. */
export default function PublishInsightsCard({
    stats,
    avg5,
    peak,
    targetFps,
    uplinkMbps,
    publishing,
}: PublishInsightsCardProps) {
    // Spinner while the encoder is starting; plain dash when idle.
    const dash: ReactNode = publishing && !stats ? (
        <IconLoader2 size={14} className="animate-spin text-muted-foreground" />
    ) : (
        "–"
    );

    return (
        <Card>
            <CardHeader>
                <CardTitle className="text-base">Publish insights</CardTitle>
            </CardHeader>
            <CardContent>
                <Table>
                    <TableHeader>
                        <TableRow>
                            <HeadTip text="True instantaneous bitrate: Δbytes/Δtime between the last two progress reports the publish engine emits. The engine's own bitrate figure is a cumulative average instead.">
                                bitrate now
                            </HeadTip>
                            <HeadTip text="Rolling mean of the instantaneous bitrate over the last 5 seconds.">
                                5 s mean
                            </HeadTip>
                            <HeadTip text="Highest instantaneous bitrate this session.">
                                peak
                            </HeadTip>
                            <HeadTip text="Cumulative average since encoder start, the figure the publish engine reports itself.">
                                cumulative
                            </HeadTip>
                            <HeadTip text="Frames the encoder produced per second vs the configured target.">
                                fps / target
                            </HeadTip>
                            <HeadTip text="Frames dropped because capture or encode could not keep up.">
                                dropped
                            </HeadTip>
                            <HeadTip text="Encoding speed relative to realtime. Below 1.000× the encoder falls behind.">
                                speed
                            </HeadTip>
                            <HeadTip text="5 s mean vs your configured uplink capacity.">
                                uplink
                            </HeadTip>
                        </TableRow>
                    </TableHeader>
                    <TableBody>
                        <TableRow>
                            <TableCell>{stats ? mbps(stats.instMbps) : dash}</TableCell>
                            <TableCell>{stats ? mbps(avg5) : dash}</TableCell>
                            <TableCell>{stats ? mbps(peak) : dash}</TableCell>
                            <TableCell>{stats ? mbps(stats.avgMbps) : dash}</TableCell>
                            <TableCell>
                                {stats ? `${stats.fps.toFixed(0)} / ${targetFps}` : dash}
                            </TableCell>
                            <TableCell>
                                {stats ? `${stats.drop} (${dropPercent(stats)}%)` : dash}
                            </TableCell>
                            <TableCell>
                                {stats ? `${stats.speed.toFixed(3)}×` : dash}
                            </TableCell>
                            <TableCell>
                                {stats ? (
                                    avg5 > uplinkMbps ? (
                                        <Badge variant="destructive">
                                            OVER: ~{avg5.toFixed(0)} / {uplinkMbps} Mbps
                                        </Badge>
                                    ) : (
                                        <Badge variant="secondary">
                                            ok ({avg5.toFixed(0)} / {uplinkMbps} Mbps)
                                        </Badge>
                                    )
                                ) : (
                                    dash
                                )}
                            </TableCell>
                        </TableRow>
                    </TableBody>
                </Table>
            </CardContent>
        </Card>
    );
}
