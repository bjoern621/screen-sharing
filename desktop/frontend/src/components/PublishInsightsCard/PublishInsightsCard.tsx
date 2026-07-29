import { ReactNode } from "react";
import { IconLoader2 } from "@tabler/icons-react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
    Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Stats } from "../../types/stream";
import { dropPercent, dupPercent, mbps } from "../../util/format";
import Tip from "../Tip/Tip";

interface PublishInsightsCardProps {
    stats: Stats | null;
    avg5: number | null;
    peak: number | null;
    targetFps: number;
    uplinkMbps: number;
    publishing: boolean;
    /** Age of the newest sample, null while none has arrived. */
    sampleAgeSec: number | null;
    /** Whether that age has passed the point where the figures still describe
     * the stream. */
    stale: boolean;
}

/** Placeholder for a figure the publish engine reported no measurement for. */
const UNMEASURED = "–";

/** How far under the target the encoder may run before the rate is called behind.
 * A sample lands a frame either side of the target on a healthy stream, since the
 * interval it is measured over does not line up with whole frames. */
const FPS_TOLERANCE = 1.5;

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

/** Formats a figure, or the placeholder when the engine did not measure it. */
function figure(value: number | null, format: (v: number) => string): string {
    return value === null ? UNMEASURED : format(value);
}

/** Live encoder insights: instantaneous vs rolling vs cumulative bitrate, fps,
 * repeated and discarded frames, speed and the uplink comparison. */
export default function PublishInsightsCard({
    stats,
    avg5,
    peak,
    targetFps,
    uplinkMbps,
    publishing,
    sampleAgeSec,
    stale,
}: PublishInsightsCardProps) {
    // Spinner while the encoder is starting; plain dash when idle. A stream that
    // reported and then went quiet keeps its figures and is marked stale, since
    // the spinner would claim it is still starting.
    const dash: ReactNode = publishing && !stats ? (
        <IconLoader2 size={14} className="animate-spin text-muted-foreground" />
    ) : (
        UNMEASURED
    );

    return (
        <Card>
            <CardHeader>
                <CardTitle className="text-base flex items-center gap-2">
                    Publish insights
                    {stale && (
                        <Tip text="The publish engine has sent no progress since then. Everything below is what it last reported, not what the stream is doing now.">
                            <Badge variant="destructive">
                                no sample for {sampleAgeSec} s
                            </Badge>
                        </Tip>
                    )}
                </CardTitle>
            </CardHeader>
            <CardContent>
                <Table>
                    <TableHeader>
                        <TableRow>
                            <HeadTip text="True instantaneous bitrate: Δbytes over the wall-clock interval between the last two progress reports the publish engine emits. The engine's own bitrate figure is a cumulative average instead.">
                                bitrate now
                            </HeadTip>
                            <HeadTip text="Rolling mean of the instantaneous bitrate over the last 5 seconds. Empties once the last sample is older than that.">
                                5 s mean
                            </HeadTip>
                            <HeadTip text="Highest instantaneous bitrate this session.">
                                peak
                            </HeadTip>
                            <HeadTip text="Cumulative average since encoder start, the figure the publish engine reports itself.">
                                cumulative
                            </HeadTip>
                            <HeadTip text="Frames the encoder produced over the last progress interval vs the configured target.">
                                fps / target
                            </HeadTip>
                            <HeadTip text="Frames the encoder repeated to hold the output rate. This is what rises when capture or encode cannot keep up: the timeline is constant-rate, so a picture that arrives late is filled in with the previous one.">
                                duplicated
                            </HeadTip>
                            <HeadTip text="Frames discarded before the encoder for arriving faster than the output rate. A different event from a repeat, and zero unless the output rate is below the capture rate.">
                                dropped
                            </HeadTip>
                            <HeadTip text="Encoding speed relative to realtime, over the run. Below 1.00× the encoder falls behind.">
                                speed
                            </HeadTip>
                            <HeadTip text="Encoded rate vs the configured target. BEHIND means the encoder is not keeping up: it does not slow the stream down, it discards the frames it cannot take, so the difference is capture that never leaves the machine. Encode capacity in the settings form measures what these settings can reach before publishing.">
                                encoder
                            </HeadTip>
                            <HeadTip text="5 s mean vs the configured uplink capacity.">
                                uplink
                            </HeadTip>
                        </TableRow>
                    </TableHeader>
                    <TableBody>
                        <TableRow
                            className={stale ? "text-muted-foreground" : undefined}
                        >
                            <TableCell>
                                {stats ? figure(stats.instMbps, mbps) : dash}
                            </TableCell>
                            <TableCell>{stats ? figure(avg5, mbps) : dash}</TableCell>
                            <TableCell>{stats ? figure(peak, mbps) : dash}</TableCell>
                            <TableCell>
                                {stats ? figure(stats.avgMbps, mbps) : dash}
                            </TableCell>
                            <TableCell>
                                {stats
                                    ? `${figure(stats.fps, v => v.toFixed(0))} / ${targetFps}`
                                    : dash}
                            </TableCell>
                            <TableCell>
                                {stats ? `${stats.dup} (${dupPercent(stats)}%)` : dash}
                            </TableCell>
                            <TableCell>
                                {stats ? `${stats.drop} (${dropPercent(stats)}%)` : dash}
                            </TableCell>
                            <TableCell>
                                {stats
                                    ? figure(stats.speed, v => `${v.toFixed(2)}×`)
                                    : dash}
                            </TableCell>
                            <TableCell>
                                {stats && stats.fps !== null ? (
                                    stats.fps < targetFps - FPS_TOLERANCE ? (
                                        <Badge variant="destructive">
                                            BEHIND: {stats.fps.toFixed(0)} /{" "}
                                            {targetFps} fps
                                        </Badge>
                                    ) : (
                                        <Badge variant="secondary">
                                            ok ({stats.fps.toFixed(0)} / {targetFps}{" "}
                                            fps)
                                        </Badge>
                                    )
                                ) : (
                                    dash
                                )}
                            </TableCell>
                            <TableCell>
                                {stats && avg5 !== null ? (
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
