import { useEffect, useRef, useState } from "react";
import {
    IconLoader2,
    IconMaximize,
    IconMinimize,
    IconPlugConnectedX,
    IconRefresh,
    IconVideo,
    IconVideoOff,
    IconVolume,
    IconVolumeOff,
    IconX,
} from "@tabler/icons-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Slider } from "@/components/ui/slider";
import { Toggle } from "@/components/ui/toggle";
import { cn } from "@/lib/utils";
import { RelayStatus, Stream } from "../../types/stream";
import { useWhep, WhepConn } from "../../hooks/useWhep";
import { useTestStreams, TEST_STREAM_NAMES } from "../../hooks/useTestStreams";
import Tip from "../Tip/Tip";

interface StreamGridPageProps {
    /** Live paths from the relay poll; the roster offers each for viewing. */
    paths: RelayStatus["paths"];
    s: Stream;
    onClose: () => void;
}

/**
 * Full-screen test page: watches live streams over WHEP in an auto-layouting
 * tile grid, Discord style. The roster connects and disconnects streams; each
 * tile offers mute, volume, hide-video and spotlight. A stream with hidden
 * video leaves the grid and moves to an audio-only strip where sound keeps
 * playing. Locally generated test streams sit next to the relay streams for
 * exercising the grid without a publisher. Closing the page tears down
 * everything.
 */
export default function StreamGridPage({ paths, s, onClose }: StreamGridPageProps) {
    const whep = useWhep(s.relayHost, s.webrtcPort);
    const test = useTestStreams();
    const [muted, setMuted] = useState<Record<string, boolean>>({});
    const [volume, setVolume] = useState<Record<string, number>>({});
    const [videoHidden, setVideoHidden] = useState<Record<string, boolean>>({});
    const [spotlight, setSpotlight] = useState<string | null>(null);

    useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.key === "Escape") onClose();
        };
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    }, [onClose]);

    const conns = { ...whep.conns, ...test.conns };

    const disconnect = (name: string) => {
        if (name in test.conns) test.disconnect(name);
        else whep.disconnect(name);
        setSpotlight(prev => (prev === name ? null : prev));
    };

    const reconnect = (name: string) => {
        if (TEST_STREAM_NAMES.includes(name)) test.connect(name);
        else void whep.connect(name);
    };

    const setStreamVolume = (name: string, v: number) => {
        setVolume(p => ({ ...p, [name]: v }));
        // Adjusting the volume expresses intent to hear the stream.
        if (v > 0) setMuted(p => ({ ...p, [name]: false }));
    };

    const toggleVideo = (name: string) => {
        const hidden = !(videoHidden[name] ?? false);
        if (hidden && spotlight === name) setSpotlight(null);
        setVideoHidden(p => ({ ...p, [name]: hidden }));
    };

    const tileNames = Object.keys(conns);
    const audioNames = tileNames.filter(n => videoHidden[n] ?? false);
    const videoNames = tileNames.filter(n => !(videoHidden[n] ?? false));
    const spotlit = spotlight && conns[spotlight] ? spotlight : null;
    const cols = Math.ceil(Math.sqrt(videoNames.length));
    const rows = Math.ceil(videoNames.length / Math.max(cols, 1));

    const tile = (name: string) => (
        <Tile
            key={name}
            name={name}
            conn={conns[name]}
            muted={muted[name] ?? false}
            volume={volume[name] ?? 1}
            spotlit={spotlit === name}
            onToggleMute={() => setMuted(p => ({ ...p, [name]: !(p[name] ?? false) }))}
            onAutoMuted={() => setMuted(p => ({ ...p, [name]: true }))}
            onVolume={v => setStreamVolume(name, v)}
            onHideVideo={() => toggleVideo(name)}
            onToggleSpotlight={() =>
                setSpotlight(prev => (prev === name ? null : name))
            }
            onDisconnect={() => disconnect(name)}
            onRetry={() => reconnect(name)}
        />
    );

    return (
        <div className="fixed inset-0 z-50 bg-background text-foreground">
            <div className="mx-auto flex h-full max-w-7xl flex-col gap-4 p-4">
                <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                        <h1 className="text-xl font-semibold">Stream grid</h1>
                        <Badge variant="outline">WHEP</Badge>
                    </div>
                    <Button variant="outline" size="sm" onClick={onClose}>
                        <IconX size={16} /> Close
                    </Button>
                </div>

                <Card>
                    <CardHeader>
                        <CardTitle className="text-base flex items-center gap-2">
                            Streams
                            {tileNames.length > 0 && (
                                <Badge variant="secondary">
                                    {tileNames.length} watching
                                </Badge>
                            )}
                        </CardTitle>
                    </CardHeader>
                    <CardContent className="space-y-2">
                        <div className="flex flex-wrap items-center gap-2 text-sm">
                            <span className="text-muted-foreground">relay</span>
                            {paths.length === 0 && (
                                <span className="text-muted-foreground">
                                    no streams on the relay
                                </span>
                            )}
                            {paths.map(p => {
                                const active = p.name in whep.conns;
                                return (
                                    <Toggle
                                        key={p.name}
                                        size="sm"
                                        variant="outline"
                                        pressed={active}
                                        disabled={!p.ready && !active}
                                        onPressedChange={() =>
                                            active
                                                ? disconnect(p.name)
                                                : void whep.connect(p.name)
                                        }
                                        className="rounded-full aria-pressed:border-primary/50 aria-pressed:bg-primary/15 aria-pressed:text-foreground"
                                    >
                                        <span
                                            className={cn(
                                                "size-1.5 rounded-full",
                                                active
                                                    ? "animate-pulse bg-primary"
                                                    : "bg-muted-foreground/50"
                                            )}
                                        />
                                        {p.name}
                                        <span className="text-[0.625rem] text-muted-foreground">
                                            {p.ready
                                                ? `${p.inMbps.toFixed(1)} Mbps`
                                                : "starting"}
                                        </span>
                                    </Toggle>
                                );
                            })}
                        </div>
                        <div className="flex flex-wrap items-center gap-2 text-sm">
                            <span className="text-muted-foreground">test</span>
                            {TEST_STREAM_NAMES.map(name => {
                                const active = name in test.conns;
                                return (
                                    <Toggle
                                        key={name}
                                        size="sm"
                                        variant="outline"
                                        pressed={active}
                                        onPressedChange={() =>
                                            active
                                                ? disconnect(name)
                                                : test.connect(name)
                                        }
                                        className="rounded-full border-dashed aria-pressed:border-amber-500/50 aria-pressed:bg-amber-500/10 aria-pressed:text-foreground"
                                    >
                                        <span
                                            className={cn(
                                                "size-1.5 rounded-full",
                                                active
                                                    ? "animate-pulse bg-amber-400"
                                                    : "bg-muted-foreground/50"
                                            )}
                                        />
                                        {name}
                                    </Toggle>
                                );
                            })}
                        </div>
                    </CardContent>
                </Card>

                {audioNames.length > 0 && (
                    <Card>
                        <CardHeader>
                            <CardTitle className="text-base flex items-center gap-2">
                                Audio only
                                <Badge variant="secondary">
                                    {audioNames.length}
                                </Badge>
                            </CardTitle>
                        </CardHeader>
                        <CardContent className="flex flex-wrap gap-2">
                            {audioNames.map(name => (
                                <AudioOnlyChip
                                    key={name}
                                    name={name}
                                    conn={conns[name]}
                                    muted={muted[name] ?? false}
                                    volume={volume[name] ?? 1}
                                    onToggleMute={() =>
                                        setMuted(p => ({
                                            ...p,
                                            [name]: !(p[name] ?? false),
                                        }))
                                    }
                                    onAutoMuted={() =>
                                        setMuted(p => ({ ...p, [name]: true }))
                                    }
                                    onVolume={v => setStreamVolume(name, v)}
                                    onShowVideo={() => toggleVideo(name)}
                                    onDisconnect={() => disconnect(name)}
                                />
                            ))}
                        </CardContent>
                    </Card>
                )}

                <main className="min-h-0 flex-1">
                    {videoNames.length === 0 ? (
                        <div className="flex h-full items-center justify-center">
                            <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed px-12 py-10 text-muted-foreground">
                                <div className="flex size-12 items-center justify-center rounded-full bg-muted">
                                    <IconVideo size={22} />
                                </div>
                                <span className="text-sm">
                                    {tileNames.length === 0
                                        ? "pick a stream above to start watching"
                                        : "all connected streams are audio-only"}
                                </span>
                            </div>
                        </div>
                    ) : spotlit ? (
                        <div className="flex h-full flex-col gap-3">
                            <div className="min-h-0 flex-1">{tile(spotlit)}</div>
                            {videoNames.length > 1 && (
                                <div className="flex h-28 justify-center gap-3">
                                    {videoNames
                                        .filter(n => n !== spotlit)
                                        .map(n => (
                                            <div
                                                key={n}
                                                className="aspect-video h-full"
                                            >
                                                {tile(n)}
                                            </div>
                                        ))}
                                </div>
                            )}
                        </div>
                    ) : (
                        // Flex-wrap instead of CSS grid so an incomplete last row
                        // centers its tiles ("centered last row" / Meet-style grid).
                        <div className="flex h-full flex-wrap content-center justify-center gap-3">
                            {videoNames.map(name => (
                                <div
                                    key={name}
                                    style={{
                                        width: `calc((100% - ${(cols - 1) * 0.75}rem) / ${cols})`,
                                        height: `calc((100% - ${(rows - 1) * 0.75}rem) / ${rows})`,
                                    }}
                                >
                                    {tile(name)}
                                </div>
                            ))}
                        </div>
                    )}
                </main>
            </div>
        </div>
    );
}

/** Drives a media element from a WHEP connection: attaches the stream, starts
 * playback (falling back to muted when autoplay with sound is blocked, and
 * reporting it), and applies mute/volume changes. */
function useMediaSink(
    conn: WhepConn,
    muted: boolean,
    volume: number,
    onAutoMuted: () => void
) {
    const ref = useRef<HTMLVideoElement>(null);
    const autoMutedRef = useRef(onAutoMuted);
    autoMutedRef.current = onAutoMuted;

    useEffect(() => {
        const v = ref.current;
        if (!v) return;
        v.srcObject = conn.stream;
        v.play().catch(() => {
            autoMutedRef.current();
            v.muted = true;
            void v.play().catch(() => {});
        });
    }, [conn.stream]);

    useEffect(() => {
        const v = ref.current;
        if (!v) return;
        v.muted = muted;
        v.volume = volume;
    }, [muted, volume]);

    return ref;
}

interface VolumeControlProps {
    muted: boolean;
    volume: number;
    onToggleMute: () => void;
    onVolume: (v: number) => void;
}

/** Mute button that reveals the volume slider while hovered or focused. */
function VolumeControl({
    muted,
    volume,
    onToggleMute,
    onVolume,
}: VolumeControlProps) {
    return (
        <div className="group/vol flex items-center">
            <Tip text={muted ? "unmute" : "mute"}>
                <Button variant="ghost" size="icon" onClick={onToggleMute}>
                    {muted ? <IconVolumeOff /> : <IconVolume />}
                </Button>
            </Tip>
            <div className="w-0 overflow-hidden opacity-0 transition-all duration-200 group-focus-within/vol:w-20 group-focus-within/vol:opacity-100 group-hover/vol:w-20 group-hover/vol:opacity-100">
                <div className="w-20 px-1.5">
                    <Slider
                        value={[volume]}
                        min={0}
                        max={1}
                        step={0.05}
                        onValueChange={v =>
                            onVolume(Array.isArray(v) ? (v[0] ?? 1) : v)
                        }
                    />
                </div>
            </div>
        </div>
    );
}

interface TileProps {
    name: string;
    conn: WhepConn;
    muted: boolean;
    volume: number;
    spotlit: boolean;
    onToggleMute: () => void;
    /** Reports that playback had to fall back to muted (autoplay policy). */
    onAutoMuted: () => void;
    onVolume: (v: number) => void;
    onHideVideo: () => void;
    onToggleSpotlight: () => void;
    onDisconnect: () => void;
    onRetry: () => void;
}

/** One stream tile: the video element plus a hover control bar. */
function Tile({
    name,
    conn,
    muted,
    volume,
    spotlit,
    onToggleMute,
    onAutoMuted,
    onVolume,
    onHideVideo,
    onToggleSpotlight,
    onDisconnect,
    onRetry,
}: TileProps) {
    const videoRef = useMediaSink(conn, muted, volume, onAutoMuted);

    return (
        <div
            className={cn(
                "group relative h-full w-full overflow-hidden rounded-lg bg-black ring-1 ring-foreground/10 transition-shadow",
                spotlit && "ring-2 ring-primary/60"
            )}
        >
            <video
                ref={videoRef}
                autoPlay
                playsInline
                className="absolute inset-0 h-full w-full object-contain"
            />
            {conn.state === "connecting" && (
                <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 text-muted-foreground">
                    <IconLoader2 size={20} className="animate-spin" />
                    <span className="text-xs">connecting</span>
                </div>
            )}
            {conn.state === "failed" && (
                <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 p-4 text-center">
                    <div className="flex size-10 items-center justify-center rounded-full bg-destructive/15 text-destructive">
                        <IconPlugConnectedX size={20} />
                    </div>
                    <span className="text-sm text-destructive">
                        {conn.error ?? "connection failed"}
                    </span>
                    <Button size="sm" variant="outline" onClick={onRetry}>
                        <IconRefresh /> retry
                    </Button>
                </div>
            )}

            <span className="absolute left-2 top-2 flex items-center gap-1.5 rounded-md bg-black/60 px-2.5 py-1 text-sm font-medium text-white">
                {name}
                {muted && <IconVolumeOff size={16} />}
            </span>

            <div className="absolute bottom-2 left-1/2 flex -translate-x-1/2 translate-y-1 items-center gap-1 rounded-lg border bg-background/80 px-1.5 py-1 opacity-0 shadow-lg backdrop-blur-md transition-all group-hover:translate-y-0 group-hover:opacity-100 has-[[aria-expanded=true]]:translate-y-0 has-[[aria-expanded=true]]:opacity-100">
                <VolumeControl
                    muted={muted}
                    volume={volume}
                    onToggleMute={onToggleMute}
                    onVolume={onVolume}
                />
                <Tip text="hide video (audio keeps playing)">
                    <Button variant="ghost" size="icon" onClick={onHideVideo}>
                        <IconVideoOff />
                    </Button>
                </Tip>
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

interface AudioOnlyChipProps {
    name: string;
    conn: WhepConn;
    muted: boolean;
    volume: number;
    onToggleMute: () => void;
    /** Reports that playback had to fall back to muted (autoplay policy). */
    onAutoMuted: () => void;
    onVolume: (v: number) => void;
    onShowVideo: () => void;
    onDisconnect: () => void;
}

/** One audio-only stream: a hidden media element keeps the sound playing while
 * the stream stays out of the video grid. */
function AudioOnlyChip({
    name,
    conn,
    muted,
    volume,
    onToggleMute,
    onAutoMuted,
    onVolume,
    onShowVideo,
    onDisconnect,
}: AudioOnlyChipProps) {
    const mediaRef = useMediaSink(conn, muted, volume, onAutoMuted);

    return (
        <div className="flex items-center gap-1.5 rounded-md border px-2 py-1">
            <video ref={mediaRef} autoPlay playsInline className="hidden" />
            <div className="flex size-6 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">
                {name[0]?.toUpperCase()}
            </div>
            <span className="text-sm font-medium">{name}</span>
            {conn.state === "connecting" && (
                <IconLoader2 size={14} className="animate-spin text-muted-foreground" />
            )}
            <VolumeControl
                muted={muted}
                volume={volume}
                onToggleMute={onToggleMute}
                onVolume={onVolume}
            />
            <Tip text="show video">
                <Button variant="ghost" size="icon" onClick={onShowVideo}>
                    <IconVideo />
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
    );
}
