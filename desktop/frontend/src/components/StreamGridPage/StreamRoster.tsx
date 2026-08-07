import { IconAlertTriangle, IconLoader2 } from "@tabler/icons-react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Toggle } from "@/components/ui/toggle";
import { cn } from "@/lib/utils";
import { RelayStatus } from "../../types/stream";
import { StreamSink } from "../../types/sink";
import { useSinkSnapshot } from "../../hooks/useSinkView";

type RelayPath = RelayStatus["paths"][number];

interface StreamRosterProps {
    paths: RelayStatus["paths"];
    /** The connected sinks, by stream name. */
    sinks: Record<string, StreamSink>;
    watching: number;
    onToggle: (name: string) => void;
}

/** The roster card: one toggle per relay path, connecting or disconnecting its
 * tile and reporting that tile's connection state back on the chip. */
export default function StreamRoster({
    paths,
    sinks,
    watching,
    onToggle,
}: StreamRosterProps) {
    return (
        <Card>
            <CardHeader>
                <CardTitle className="text-base flex items-center gap-2">
                    Streams
                    {watching > 0 && (
                        <Badge variant="secondary">{watching} watching</Badge>
                    )}
                </CardTitle>
            </CardHeader>
            <CardContent>
                <div className="flex flex-wrap items-center gap-2 text-sm">
                    {paths.length === 0 && (
                        <span className="text-muted-foreground">
                            No streams on the relay
                        </span>
                    )}
                    {paths.map(p => (
                        <RosterChip
                            key={p.name}
                            path={p}
                            sink={sinks[p.name]}
                            onToggle={() => onToggle(p.name)}
                        />
                    ))}
                </div>
            </CardContent>
        </Card>
    );
}

interface RosterChipProps {
    path: RelayPath;
    /** The sink watching this path, absent while it is not being watched. */
    sink?: StreamSink;
    onToggle: () => void;
}

/**
 * One relay path as a toggle. The status dot is the tile's connection in
 * miniature, so the click has visible effect from the moment it lands: spinner
 * while the sink connects, pulsing dot once video runs, warning triangle when it
 * failed.
 */
function RosterChip({ path, sink, onToggle }: RosterChipProps) {
    const snapshot = useSinkSnapshot(sink);
    const state = snapshot?.state;

    return (
        <Toggle
            size="sm"
            variant="outline"
            pressed={sink !== undefined}
            disabled={!path.ready && sink === undefined}
            onPressedChange={onToggle}
            // A watched chip carries its own on-fill, so it states its own
            // on-hover too: the toggle's default one is written as
            // aria-pressed:hover: and would otherwise outrank the tint below and
            // grey the chip out under the pointer.
            className={cn(
                "rounded-full aria-pressed:border-primary/50 aria-pressed:bg-primary/15 aria-pressed:text-foreground aria-pressed:hover:bg-primary/25",
                state === "failed" &&
                    "aria-pressed:border-destructive/50 aria-pressed:bg-destructive/10 aria-pressed:hover:bg-destructive/20"
            )}
        >
            {state === "connecting" ? (
                <IconLoader2 className="animate-spin" />
            ) : state === "failed" ? (
                <IconAlertTriangle className="text-destructive" />
            ) : (
                <span
                    className={cn(
                        "size-1.5 rounded-full",
                        state === "connected"
                            ? "animate-pulse bg-primary"
                            : "bg-muted-foreground/50"
                    )}
                />
            )}
            {path.name}
            <span className="text-[0.625rem] text-muted-foreground">
                {path.ready ? `${path.inMbps.toFixed(1)} Mbps` : "starting"}
            </span>
        </Toggle>
    );
}
