import { IconLoader2, IconPlugConnectedX, IconVideo } from "@tabler/icons-react";
import { Button } from "@/components/ui/button";
import { StreamSink } from "../../types/sink";
import { useSinkView } from "../../hooks/useSinkView";
import Tip from "../Tip/Tip";
import VolumeControl from "./VolumeControl";

interface AudioOnlyChipProps {
    sink: StreamSink;
    onShowVideo: () => void;
    onDisconnect: () => void;
}

/** One audio-only stream: the sink stays mounted in a zero-size hidden host so
 * its audio keeps playing while it sits out of the video grid. */
export default function AudioOnlyChip({
    sink,
    onShowVideo,
    onDisconnect,
}: AudioOnlyChipProps) {
    const { containerRef, snapshot } = useSinkView(sink);

    return (
        <div className="flex items-center gap-1.5 rounded-md border px-2 py-1">
            <div
                ref={containerRef}
                className="pointer-events-none absolute size-0 overflow-hidden"
            />
            <div className="flex size-6 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">
                {sink.name[0]?.toUpperCase()}
            </div>
            <span className="text-sm font-medium">{sink.name}</span>
            {snapshot.state === "connecting" && (
                <IconLoader2
                    size={14}
                    className="animate-spin text-muted-foreground"
                />
            )}
            {snapshot.audio && (
                <VolumeControl
                    muted={snapshot.audio.muted}
                    volume={snapshot.audio.volume}
                    onToggleMute={() =>
                        sink.audio?.setMuted(!snapshot.audio!.muted)
                    }
                    onVolume={v => sink.audio?.setVolume(v)}
                />
            )}
            <Tip text="Put this stream's picture back in the grid. The audio never stopped.">
                <Button variant="ghost" size="icon" onClick={onShowVideo}>
                    <IconVideo />
                </Button>
            </Tip>
            <Tip text="Stop watching this stream and close it. The stream keeps running at the relay.">
                <Button
                    variant="ghost"
                    size="icon"
                    className="text-destructive hover:text-destructive"
                    onClick={onDisconnect}
                >
                    <IconPlugConnectedX />
                </Button>
            </Tip>
        </div>
    );
}
