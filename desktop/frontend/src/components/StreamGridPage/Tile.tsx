import { useCallback, useEffect, useRef, useState } from "react";
import {
    IconInfoCircle,
    IconMaximize,
    IconMinimize,
    IconPictureInPicture,
    IconPlugConnectedX,
    IconRefresh,
    IconVideoOff,
    IconVolumeOff,
} from "@tabler/icons-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { StreamSink } from "../../types/sink";
import { useSinkView } from "../../hooks/useSinkView";
import { useSinkStats } from "../../hooks/useSinkStats";
import { usePictureInPicture } from "../../hooks/usePictureInPicture";
import Tip from "../Tip/Tip";
import VolumeControl from "./VolumeControl";
import TransportBadge from "./TransportBadge";
import TileLoading from "./TileLoading";
import StreamStatsOverlay, { RelayStat } from "./StreamStatsOverlay";

interface TileProps {
    sink: StreamSink;
    spotlit: boolean;
    relay?: RelayStat;
    onHideVideo: () => void;
    onToggleSpotlight: () => void;
    onDisconnect: () => void;
    onRetry: () => void;
}

/** One stream tile. The sink mounts its own video/canvas into the surface
 * container, so the tile is decoder-agnostic; the chrome, badges and stats
 * overlay sit above it. */
export default function Tile({
    sink,
    spotlit,
    relay,
    onHideVideo,
    onToggleSpotlight,
    onDisconnect,
    onRetry,
}: TileProps) {
    const { containerRef, snapshot } = useSinkView(sink);
    const [statsOpen, setStatsOpen] = useState(false);
    const stats = useSinkStats(sink, statsOpen);
    const pip = usePictureInPicture();
    const tileRef = useRef<HTMLDivElement>(null);
    const [codec, setCodec] = useState<string>();

    // Probe once (then back off) after connecting to label the badge with the
    // negotiated codec, without polling while the overlay is closed.
    useEffect(() => {
        if (snapshot.state !== "connected") {
            setCodec(undefined);
            return;
        }
        let alive = true;
        let timer = 0;
        const probe = async () => {
            const s = await sink.stats();
            if (!alive) return;
            if (s?.codec) setCodec(s.codec);
            else timer = window.setTimeout(() => void probe(), 1500);
        };
        void probe();
        return () => {
            alive = false;
            clearTimeout(timer);
        };
    }, [snapshot.state, sink]);

    const audio = snapshot.audio;

    const togglePip = useCallback(() => {
        if (tileRef.current) void pip.toggle(tileRef.current);
    }, [pip]);

    return (
        <div
            ref={tileRef}
            className={cn(
                "group relative h-full w-full animate-in fade-in zoom-in-95 overflow-hidden rounded-lg bg-black ring-1 ring-foreground/10 transition-shadow duration-300",
                spotlit && "ring-2 ring-primary/60"
            )}
        >
            {/* The surface holds decoded output before the sink reports
                connected, so it fades in rather than cutting from skeleton to
                video. */}
            <div
                ref={containerRef}
                className={cn(
                    "absolute inset-0 transition-opacity duration-500",
                    snapshot.state === "connected" ? "opacity-100" : "opacity-0"
                )}
            />

            {snapshot.state === "connecting" && (
                <TileLoading name={sink.name} phase={snapshot.phase} />
            )}
            {snapshot.state === "failed" && (
                <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 p-4 text-center">
                    <div className="flex size-10 items-center justify-center rounded-full bg-destructive/15 text-destructive">
                        <IconPlugConnectedX size={20} />
                    </div>
                    <span className="text-sm text-destructive">
                        {snapshot.error ?? "connection failed"}
                    </span>
                    <Button size="sm" variant="outline" onClick={onRetry}>
                        <IconRefresh /> retry
                    </Button>
                </div>
            )}

            {/* The skeleton names the stream in its centre, so the corner label
                would be the second copy while connecting. */}
            {snapshot.state !== "connecting" && (
                <span className="absolute left-2 top-2 flex items-center gap-1.5 rounded-md bg-black/60 px-2.5 py-1 text-sm font-medium text-white">
                    {sink.name}
                    {audio?.muted && <IconVolumeOff size={16} />}
                </span>
            )}
            <span className="absolute right-2 top-2">
                <TransportBadge kind={sink.kind} codec={codec} />
            </span>

            {statsOpen && <StreamStatsOverlay stats={stats} relay={relay} />}

            <div className="absolute bottom-2 left-1/2 flex -translate-x-1/2 translate-y-1 items-center gap-1 rounded-lg border bg-background/80 px-1.5 py-1 opacity-0 shadow-lg backdrop-blur-md transition-all duration-200 group-hover:translate-y-0 group-hover:opacity-100 has-[[aria-expanded=true]]:translate-y-0 has-[[aria-expanded=true]]:opacity-100">
                {audio && (
                    <VolumeControl
                        muted={audio.muted}
                        volume={audio.volume}
                        onToggleMute={() => sink.audio?.setMuted(!audio.muted)}
                        onVolume={v => sink.audio?.setVolume(v)}
                    />
                )}
                <Tip text={statsOpen ? "hide stats" : "stats"}>
                    <Button
                        variant="ghost"
                        size="icon"
                        aria-pressed={statsOpen}
                        className={cn(statsOpen && "text-primary")}
                        onClick={() => setStatsOpen(v => !v)}
                    >
                        <IconInfoCircle />
                    </Button>
                </Tip>
                {pip.supported && (
                    <Tip text={pip.active ? "exit pop-out" : "pop out"}>
                        <Button
                            variant="ghost"
                            size="icon"
                            className={cn(pip.active && "text-primary")}
                            onClick={togglePip}
                        >
                            <IconPictureInPicture />
                        </Button>
                    </Tip>
                )}
                {audio && (
                    <Tip text="hide video (audio keeps playing)">
                        <Button
                            variant="ghost"
                            size="icon"
                            onClick={onHideVideo}
                        >
                            <IconVideoOff />
                        </Button>
                    </Tip>
                )}
                <Tip text={spotlit ? "back to grid" : "spotlight"}>
                    <Button
                        variant="ghost"
                        size="icon"
                        onClick={onToggleSpotlight}
                    >
                        {spotlit ? <IconMinimize /> : <IconMaximize />}
                    </Button>
                </Tip>
                <Tip text="disconnect">
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
        </div>
    );
}
