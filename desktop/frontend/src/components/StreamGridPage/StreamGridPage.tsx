import { useEffect, useState } from "react";
import { IconVideo, IconX } from "@tabler/icons-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { RelayStatus, Stream } from "../../types/stream";
import { SinkKind } from "../../types/sink";
import { useSinks } from "../../hooks/useSinks";
import { sinkKindForTracks } from "../../util/webgrid";
import StreamRoster from "./StreamRoster";
import Tile from "./Tile";
import AudioOnlyChip from "./AudioOnlyChip";
import { RelayStat } from "./StreamStatsOverlay";

interface StreamGridPageProps {
    /** Live paths from the relay poll; the roster offers each for viewing. */
    paths: RelayStatus["paths"];
    s: Stream;
    onClose: () => void;
}

/**
 * Port of the in-app viewer service serving encoded frames over WebSocket.
 * Wired to a setting when the Go viewer service lands; a constant until then.
 */
const VIEWER_PORT = 8899;

/**
 * The web grid: a full-screen page watching live streams in an auto-layouting
 * tile grid, Discord style. Each tile decodes through a StreamSink (WHEP today,
 * WebCodecs for 4:4:4), so the grid is decoder-agnostic. The roster connects and
 * disconnects streams; a tile offers mute, volume, stats, pop-out, hide-video
 * and spotlight. A stream with hidden video moves to an audio-only strip where
 * its sound keeps playing. Closing the page tears everything down.
 */
export default function StreamGridPage({ paths, s, onClose }: StreamGridPageProps) {
    const { sinks, connect, disconnect } = useSinks({
        relayHost: s.relayHost,
        webrtcPort: s.webrtcPort,
        viewerPort: VIEWER_PORT,
    });
    const [videoHidden, setVideoHidden] = useState<Record<string, boolean>>({});
    const [spotlight, setSpotlight] = useState<string | null>(null);

    useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.key === "Escape") onClose();
        };
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    }, [onClose]);

    const close = (name: string) => {
        disconnect(name);
        setSpotlight(prev => (prev === name ? null : prev));
    };

    /** The decoder for a path, from the codecs the relay reports on it. */
    const kindOf = (name: string): SinkKind => {
        const p = paths.find(p => p.name === name);
        return sinkKindForTracks(p?.tracks ?? "");
    };

    const toggle = (name: string) =>
        name in sinks ? close(name) : connect(name, kindOf(name));

    const retry = (name: string) => connect(name, kindOf(name));

    const toggleVideo = (name: string) => {
        const hidden = !(videoHidden[name] ?? false);
        if (hidden && spotlight === name) setSpotlight(null);
        setVideoHidden(p => ({ ...p, [name]: hidden }));
    };

    const relayStat = (name: string): RelayStat | undefined => {
        const p = paths.find(p => p.name === name);
        return p
            ? { ready: p.ready, readers: p.readers, inMbps: p.inMbps, tracks: p.tracks }
            : undefined;
    };

    const tileNames = Object.keys(sinks);
    const audioNames = tileNames.filter(n => videoHidden[n] ?? false);
    const videoNames = tileNames.filter(n => !(videoHidden[n] ?? false));
    const spotlit = spotlight && sinks[spotlight] ? spotlight : null;
    const cols = Math.ceil(Math.sqrt(videoNames.length));
    const rows = Math.ceil(videoNames.length / Math.max(cols, 1));

    const tile = (name: string) => (
        <Tile
            key={name}
            sink={sinks[name]}
            spotlit={spotlit === name}
            relay={relayStat(name)}
            onHideVideo={() => toggleVideo(name)}
            onToggleSpotlight={() =>
                setSpotlight(prev => (prev === name ? null : name))
            }
            onDisconnect={() => close(name)}
            onRetry={() => retry(name)}
        />
    );

    return (
        <div className="fixed inset-0 z-50 bg-background text-foreground">
            <div className="mx-auto flex h-full max-w-7xl flex-col gap-4 p-4">
                <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                        <h1 className="text-xl font-semibold">Web grid</h1>
                        {tileNames.length > 0 && (
                            <Badge variant="secondary">
                                {tileNames.length} watching
                            </Badge>
                        )}
                    </div>
                    <Button variant="outline" size="sm" onClick={onClose}>
                        <IconX size={16} /> Close
                    </Button>
                </div>

                <StreamRoster
                    paths={paths}
                    sinks={sinks}
                    watching={tileNames.length}
                    onToggle={toggle}
                />

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
                                    sink={sinks[name]}
                                    onShowVideo={() => toggleVideo(name)}
                                    onDisconnect={() => close(name)}
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
                        // centers its tiles (Meet-style centered last row).
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
