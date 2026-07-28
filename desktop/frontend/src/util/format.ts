import { Stats } from "../types/stream";

/** Formats a Mbit/s figure with one decimal and unit. */
export function mbps(value: number): string {
    return `${value.toFixed(1)} Mbps`;
}

/** Percentage of the encoded frames that were repeats, as a string. Duplicates
 * were encoded, so they are already part of the frame count. */
export function dupPercent(stats: Stats): string {
    if (stats.frame <= 0) {
        return "0";
    }
    return ((100 * stats.dup) / stats.frame).toFixed(2);
}

/** Percentage of the captured frames that never reached the encoder, as a
 * string. Dropped frames were not encoded, so the frame count excludes them and
 * the two counts together are what arrived. */
export function dropPercent(stats: Stats): string {
    const arrived = stats.frame + stats.drop;
    if (arrived <= 0) {
        return "0";
    }
    return ((100 * stats.drop) / arrived).toFixed(2);
}
