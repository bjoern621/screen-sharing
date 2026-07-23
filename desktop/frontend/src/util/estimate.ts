import { Stream } from "../types/stream";
import { CHROMA_META, CODEC_META, Chroma, Codec } from "./domain";

/** A predicted bitrate for the current settings, before publishing. */
export interface BitrateEstimate {
    /** "fixed" = CBR constant; "range" = quality VBR; "lossless" = unbounded. */
    kind: "fixed" | "range" | "lossless";
    lowMbps: number;
    highMbps: number;
    note: string;
}

// Bits/pixel/frame for H.264 4:2:0 at CQ 23 on mixed content: the anchor of the
// quality model. Each 6 CQ steps roughly halves or doubles the bitrate.
const QUALITY_ANCHOR_BPP = 0.07;
const QUALITY_ANCHOR_CQ = 23;
const CQ_STEP = 6;

// Content spread around the nominal quality bitrate: static desktop -> motion.
const MOTION_LOW = 0.4;
const MOTION_HIGH = 2.5;

// Lossless spread: a near-static screen compresses hard; heavy motion nears raw.
const LOSSLESS_LOW = 0.06;
const LOSSLESS_HIGH = 0.55;

/**
 * Estimates the bitrate the current settings will produce for a width×height
 * source. Heuristic and content-dependent - it returns a range, not a promise.
 * Returns null when the resolution is unknown (width/height 0).
 */
export function estimateBitrate(
    s: Stream,
    width: number,
    height: number
): BitrateEstimate | null {
    if (width <= 0 || height <= 0) {
        return null;
    }
    const pixelRate = width * height * s.fps; // pixels per second

    if (s.mode === "latency") {
        return {
            kind: "fixed",
            lowMbps: s.bitrateM,
            highMbps: s.bitrateM,
            note: "CBR: held constant at the bitrate bound",
        };
    }

    const codec = CODEC_META[s.codec as Codec]?.efficiency ?? 1.0;
    const chroma = CHROMA_META[s.chroma as Chroma]?.weight ?? 1.0;

    if (s.mode === "lossless") {
        const raw = CHROMA_META[s.chroma as Chroma]?.rawBpp ?? 24;
        return {
            kind: "lossless",
            lowMbps: (pixelRate * raw * LOSSLESS_LOW) / 1e6,
            highMbps: (pixelRate * raw * LOSSLESS_HIGH) / 1e6,
            note: "lossless: content-dependent, peaks toward raw on motion",
        };
    }

    // quality (VBR + constant quantizer)
    const bpp =
        QUALITY_ANCHOR_BPP *
        Math.pow(2, (QUALITY_ANCHOR_CQ - s.cq) / CQ_STEP) *
        codec *
        chroma;
    const nominal = (pixelRate * bpp) / 1e6;
    let low = nominal * MOTION_LOW;
    let high = nominal * MOTION_HIGH;
    let note = `quality VBR at CQ ${s.cq}`;
    if (s.bitrateM > 0) {
        low = Math.min(low, s.bitrateM);
        high = Math.min(high, s.bitrateM);
        note += `, capped by the ${s.bitrateM} Mbit/s ceiling`;
    }
    return { kind: "range", lowMbps: low, highMbps: high, note };
}

/** Formats an estimate as a compact Mbit/s figure or range. */
export function formatEstimate(e: BitrateEstimate): string {
    const n = (v: number) => (v >= 10 ? Math.round(v).toString() : v.toFixed(1));
    if (e.kind === "fixed") {
        return `${n(e.lowMbps)} Mbit/s`;
    }
    return `~${n(e.lowMbps)}–${n(e.highMbps)} Mbit/s`;
}
