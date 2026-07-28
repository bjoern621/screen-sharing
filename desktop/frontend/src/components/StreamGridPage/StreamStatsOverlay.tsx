import { SinkStats } from "../../types/sink";

/** Relay-side figures for the same stream, from the Live() poll. */
export interface RelayStat {
    ready: boolean;
    readers: number;
    inMbps: number;
    tracks: string;
}

interface StreamStatsOverlayProps {
    stats: SinkStats | null;
    relay?: RelayStat;
}

/** Placeholder for a figure the sink cannot report, as the native grid prints
 * it. */
const UNKNOWN = "…";

/** A counter as text, or the placeholder where the sink took no such measurement.
 * SinkStats types the counters as numbers, so an unmeasured one arrives as NaN
 * rather than as an absent field. */
function count(n: number): string {
    return Number.isFinite(n) ? String(n) : UNKNOWN;
}

/** Separator between two figures on one row, as the native grid joins them. */
const JOIN = " · ";

/** The decode figures as the native grid prints them: the counts, and the drop
 * count's share of the frames that arrived. A raw drop count means nothing
 * without the run it happened over, so the share is what says whether decode is
 * keeping up. Sinks that take no drop measurement report NaN, and a share
 * computed from one would be a figure nobody measured. */
function framesText(decoded: number, dropped: number): string {
    const text = `${count(decoded)} decoded${JOIN}${count(dropped)} dropped`;
    if (!Number.isFinite(decoded) || !Number.isFinite(dropped)) {
        return text;
    }
    const arrived = decoded + dropped;
    if (arrived <= 0) {
        return text;
    }
    return `${text} (${((100 * dropped) / arrived).toFixed(1)}%)`;
}

function Row({ label, value }: { label: string; value: string }) {
    return (
        <div className="flex justify-between gap-4">
            <span className="text-white/50">{label}</span>
            <span className="tabular-nums">{value}</span>
        </div>
    );
}

/** Per-stream detail overlay ("nerd stats"), toggled from the tile chrome. Shows
 * the decode figures the sink measures and the relay figures for the same path,
 * so what the viewer decodes and what the relay ingests sit side by side.
 * The rows carry the native grid's keys in its order, stream before pixels
 * before decode, so a figure is called the same thing on both grids. */
export default function StreamStatsOverlay({
    stats,
    relay,
}: StreamStatsOverlayProps) {
    return (
        <div className="absolute left-2 top-11 z-10 w-72 animate-in fade-in slide-in-from-top-1 rounded-md bg-black/75 p-2.5 font-mono text-[0.6875rem] leading-relaxed text-white shadow-lg backdrop-blur-sm">
            {stats ? (
                <>
                    {/* transport is the watch leg this tile receives over, the
                     * same meaning the row carries in the native grid. The publish
                     * leg lives in the settings form and is not observable here. */}
                    <Row
                        label="transport"
                        value={stats.transport || UNKNOWN}
                    />
                    {stats.latencyMs !== undefined && (
                        <Row
                            label="latency"
                            value={`${stats.latencyMs.toFixed(0)} ms`}
                        />
                    )}
                    <Row
                        label="resolution"
                        value={`${stats.width}×${stats.height}`}
                    />
                    <Row label="codec" value={stats.codec || UNKNOWN} />
                    <Row
                        label="bitrate"
                        value={`${stats.bitrateMbps.toFixed(1)} Mbps`}
                    />
                    <Row label="decoder" value={stats.decoder || UNKNOWN} />
                    <Row label="fps" value={stats.fps.toFixed(1)} />
                    <Row
                        label="frames"
                        value={framesText(stats.framesDecoded, stats.framesDropped)}
                    />
                    {stats.jitterMs !== undefined && (
                        <Row
                            label="jitter"
                            value={`${stats.jitterMs.toFixed(1)} ms`}
                        />
                    )}
                    {stats.packetsLost !== undefined && (
                        <Row label="lost" value={String(stats.packetsLost)} />
                    )}
                </>
            ) : (
                <div className="text-white/50">measuring…</div>
            )}
            {relay && (
                <>
                    <div className="my-1.5 border-t border-white/15" />
                    <div className="mb-1 text-white/40">relay</div>
                    <Row
                        label="ingest"
                        value={
                            relay.ready
                                ? `${relay.inMbps.toFixed(1)} Mbps`
                                : "starting"
                        }
                    />
                    <Row label="readers" value={String(relay.readers)} />
                    <Row label="tracks" value={relay.tracks || UNKNOWN} />
                </>
            )}
        </div>
    );
}
