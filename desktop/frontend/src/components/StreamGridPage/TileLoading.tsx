import { IconLoader2 } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { SinkPhase, SINK_PHASES } from "../../types/sink";

/** What each connect step is waiting for, in the viewer's terms. */
const PHASE_LABEL: Record<SinkPhase, string> = {
    requesting: "asking the relay for the stream",
    negotiating: "agreeing on a transport",
    buffering: "waiting for the first frame",
};

interface TileLoadingProps {
    name: string;
    phase: SinkPhase;
}

/**
 * The skeleton a tile shows between the click and the first decoded frame.
 * Every step is named and the step bar advances through them, so a connection
 * that stalls says where it stalled instead of spinning anonymously.
 */
export default function TileLoading({ name, phase }: TileLoadingProps) {
    const reached = SINK_PHASES.indexOf(phase);

    return (
        <div className="absolute inset-0 overflow-hidden bg-gradient-to-b from-muted/20 to-transparent">
            <div className="absolute inset-0 animate-shimmer bg-gradient-to-r from-transparent via-white/[0.07] to-transparent" />
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 p-4 text-center">
                <IconLoader2 size={22} className="animate-spin text-white/70" />
                <span className="max-w-full truncate text-sm font-medium text-white/90">
                    {name}
                </span>
                <div className="flex w-32 gap-1">
                    {SINK_PHASES.map((p, i) => (
                        <span
                            key={p}
                            className={cn(
                                "h-1 flex-1 rounded-full bg-white/15",
                                i < reached && "bg-white/60",
                                i === reached && "animate-pulse bg-white/60"
                            )}
                        />
                    ))}
                </div>
                <span className="text-xs text-white/60">
                    {PHASE_LABEL[phase]}
                </span>
            </div>
        </div>
    );
}
