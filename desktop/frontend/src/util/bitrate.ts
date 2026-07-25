/** A cumulative byte counter read at a point in time, for bitrate deltas. */
export interface ByteSample {
    bytes: number;
    /** Milliseconds, from the same clock across samples (getStats timestamps). */
    timestamp: number;
}

/**
 * Bitrate in Mbit/s between two cumulative byte samples. Returns 0 when the
 * samples do not straddle a positive time span or bytes did not advance, so a
 * repeated or reset counter reads as 0 rather than a spike.
 */
export function bitrateMbps(prev: ByteSample, cur: ByteSample): number {
    const dtMs = cur.timestamp - prev.timestamp;
    const dBytes = cur.bytes - prev.bytes;
    if (dtMs <= 0 || dBytes <= 0) return 0;
    return (dBytes * 8) / (dtMs / 1000) / 1_000_000;
}
