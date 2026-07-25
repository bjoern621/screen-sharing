import { Stream } from "../types/stream";
import {
    Capability, CHROMA_META, Chroma, FORMAT_META, cqMax, formatOf,
} from "./domain";

/** A predicted bitrate for the current settings, before publishing. */
export interface BitrateEstimate {
    /** "fixed" = CBR/ABR average; "range" = VBR/CRF spread; "lossless" = unbounded. */
    kind: "fixed" | "range" | "lossless";
    lowMbps: number;
    highMbps: number;
    note: string;
}

// Bits/pixel/frame for H.264 4:2:0 at CQ 23 on mixed content: the anchor of the
// quality model. Each 6 CQ steps roughly halves or doubles the bitrate. The
// anchor sits on the 51-point scale, so a codec that counts further (libvpx VP9
// reaches 63) has its quantizer mapped onto that scale first.
const QUALITY_ANCHOR_BPP = 0.07;
const QUALITY_ANCHOR_CQ = 23;
const CQ_STEP = 6;
const ANCHOR_CQ_MAX = 51;

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
    height: number,
    caps: Capability[] | null = null
): BitrateEstimate | null {
    if (width <= 0 || height <= 0) {
        return null;
    }
    const pixelRate = width * height * s.fps; // pixels per second

    if (s.mode === "cbr") {
        return {
            kind: "fixed",
            lowMbps: s.bitrateM,
            highMbps: s.bitrateM,
            note: "CBR: held constant at the bitrate target",
        };
    }
    if (s.mode === "abr") {
        return {
            kind: "fixed",
            lowMbps: s.bitrateM,
            highMbps: s.bitrateM,
            note: "ABR: averages toward the target, bursts uncapped on motion",
        };
    }
    if (s.mode === "vbr") {
        const high = Math.max(s.maxrateM, s.bitrateM);
        return {
            kind: "range",
            lowMbps: s.bitrateM,
            highMbps: high,
            note: `VBR: averages toward ${s.bitrateM}, bursts up to ${high} Mbit/s`,
        };
    }

    const codec = FORMAT_META[formatOf(s.codec, caps) ?? "h264"]?.efficiency ?? 1.0;
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

    // crf (constant quality): quality-driven, no bitrate bound
    const cq = (s.cq * ANCHOR_CQ_MAX) / cqMax(s.codec, caps);
    const bpp =
        QUALITY_ANCHOR_BPP *
        Math.pow(2, (QUALITY_ANCHOR_CQ - cq) / CQ_STEP) *
        codec *
        chroma;
    const nominal = (pixelRate * bpp) / 1e6;
    return {
        kind: "range",
        lowMbps: nominal * MOTION_LOW,
        highMbps: nominal * MOTION_HIGH,
        note: `CRF constant quality at CQ ${s.cq}`,
    };
}

/** Formats an estimate as a compact Mbit/s figure or range. */
export function formatEstimate(e: BitrateEstimate): string {
    const n = (v: number) => (v >= 10 ? Math.round(v).toString() : v.toFixed(1));
    if (e.kind === "fixed") {
        return `${n(e.lowMbps)} Mbit/s`;
    }
    return `~${n(e.lowMbps)}–${n(e.highMbps)} Mbit/s`;
}
