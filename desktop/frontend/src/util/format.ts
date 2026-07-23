import { Stats } from "../types/stream";

/** Formats a Mbit/s figure with one decimal and unit. */
export function mbps(value: number): string {
    return `${value.toFixed(1)} Mbps`;
}

/** Percentage of frames dropped relative to frames produced, as a string. */
export function dropPercent(stats: Stats): string {
    if (stats.frame <= 0) {
        return "0";
    }
    return ((100 * stats.drop) / (stats.frame + stats.drop)).toFixed(2);
}
