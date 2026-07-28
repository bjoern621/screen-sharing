import { Stream } from "../types/stream";

/**
 * A claim is the region of the settings space a preset stands for: every settings
 * object inside it delivers what the preset promises, and every object outside it
 * does not.
 *
 * Two operations read a claim, and both walk the axis tables below rather than a
 * hand-written condition per preset. `holds` decides whether the selector still
 * shows the preset after a field changed. `overlaps` decides whether two presets
 * could ever both apply to one settings object, which is the question the preset
 * table is held to at load: a selector has one value to show, so two claims that
 * intersect are a defect rather than a case to render.
 */

/** An inclusive bound on a numeric settings field. An absent end is unbounded. */
export interface Range {
    min?: number;
    max?: number;
}

/**
 * The region one preset covers. An axis the claim leaves out is unconstrained: the
 * preset makes no promise about it, so any value keeps the preset selected.
 */
export interface Claim {
    /** Rate-control modes the promise survives. */
    modes: string[];
    /** Pixel formats it survives, absent where the promise is about none. */
    chromas?: string[];
    fps?: Range;
    bframes?: Range;
    /** The two SRT retransmit windows added up, which is the delay budget a viewer
     * pays: each hop holds packets for its own window. */
    srtTotalMs?: Range;
}

/**
 * The axes a claim carves the settings space on, one entry per axis: where the
 * value comes from, and which part of the claim bounds it. Both operations below
 * read this table, so an axis added to `Claim` cannot be honored by one and missed
 * by the other.
 */
const ENUM_AXES: {
    of: (s: Stream) => string;
    allowed: (c: Claim) => string[] | undefined;
}[] = [
    { of: s => s.mode, allowed: c => c.modes },
    { of: s => s.chroma, allowed: c => c.chromas },
];

const RANGE_AXES: {
    of: (s: Stream) => number;
    range: (c: Claim) => Range | undefined;
}[] = [
    { of: s => s.fps, range: c => c.fps },
    { of: s => s.bframes, range: c => c.bframes },
    {
        of: s => s.srtPublishLatencyMs + s.srtWatchLatencyMs,
        range: c => c.srtTotalMs,
    },
];

function inRange(v: number, r: Range | undefined): boolean {
    return !r || ((r.min ?? -Infinity) <= v && v <= (r.max ?? Infinity));
}

function rangesMeet(a: Range | undefined, b: Range | undefined): boolean {
    return (
        Math.max(a?.min ?? -Infinity, b?.min ?? -Infinity) <=
        Math.min(a?.max ?? Infinity, b?.max ?? Infinity)
    );
}

function setsMeet(a: string[] | undefined, b: string[] | undefined): boolean {
    return !a || !b || a.some(v => b.includes(v));
}

/** Whether the settings deliver what the claim covers. */
export function holds(s: Stream, c: Claim): boolean {
    const enums = ENUM_AXES.every(a => {
        const allowed = a.allowed(c);
        return !allowed || allowed.includes(a.of(s));
    });
    return enums && RANGE_AXES.every(a => inRange(a.of(s), a.range(c)));
}

/**
 * Whether some settings object lies in both claims. Two regions miss each other as
 * soon as one axis separates them, so a pair that shares every axis overlaps.
 */
export function overlaps(a: Claim, b: Claim): boolean {
    return (
        ENUM_AXES.every(x => setsMeet(x.allowed(a), x.allowed(b))) &&
        RANGE_AXES.every(x => rangesMeet(x.range(a), x.range(b)))
    );
}
