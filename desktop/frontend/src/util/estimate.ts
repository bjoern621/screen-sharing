import { Stream } from "../types/stream";
import {
    ANCHOR_CQ_MAX, Capability, CHROMA_META, Chroma, Engine, FORMAT_META, cqMax, formatOf,
} from "./domain";

/** A predicted bitrate for the current settings, before publishing. */
export interface BitrateEstimate {
    /** "fixed" = a single average target: CBR or ABR; "range" = VBR/CRF spread;
     * "lossless" = unbounded. */
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

// Content spread around the nominal quality bitrate: static desktop -> motion.
const MOTION_LOW = 0.4;
const MOTION_HIGH = 2.5;

// Lossless spread: a near-static screen compresses hard; heavy motion nears raw.
const LOSSLESS_LOW = 0.06;
const LOSSLESS_HIGH = 0.55;

/**
 * Estimates the bitrate the current settings will produce for a width×height
 * source. Heuristic and content-dependent - it returns a range, not a promise.
 * Returns null where an input the figure rests on is unresolved: the source
 * resolution (width/height 0), or the codec, whose coding efficiency prices the
 * constant-quality range.
 *
 * `engine` is the publish engine of the selected capture backend, null while that
 * is unresolved. The constant-quality scale is read against it, since the two
 * engines set different properties and one may count further than the other.
 */
export function estimateBitrate(
    s: Stream,
    width: number,
    height: number,
    caps: Capability[] | null = null,
    engine: Engine | null = null
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
        // The ceiling is what separates VBR from ABR, so the mode is offered only
        // where the encoder has a property for it: an encoder that takes no ceiling
        // carries a mode gap and cannot be in VBR here at all. That is what lets this
        // price the burst without asking whether the ceiling survives to the command.
        const high = Math.max(s.maxrateM, s.bitrateM);
        return {
            kind: "range",
            lowMbps: s.bitrateM,
            highMbps: high,
            note: `VBR: averages toward ${s.bitrateM}, bursts up to ${high} Mbit/s`,
        };
    }

    if (s.mode === "lossless") {
        const raw = CHROMA_META[s.chroma as Chroma]?.rawBpp ?? 24;
        return {
            kind: "lossless",
            lowMbps: (pixelRate * raw * LOSSLESS_LOW) / 1e6,
            highMbps: (pixelRate * raw * LOSSLESS_HIGH) / 1e6,
            note: "lossless: content-dependent, peaks toward raw on motion",
        };
    }

    // crf (constant quality): quality-driven, no bitrate bound. The format's
    // coding efficiency prices the figure, so an unresolved codec withholds the
    // estimate as an unknown resolution does instead of being priced as H.264.
    const fmt = formatOf(s.codec, caps);
    if (!fmt) {
        return null;
    }
    const codec = FORMAT_META[fmt].efficiency;
    const chroma = CHROMA_META[s.chroma as Chroma]?.weight ?? 1.0;
    // The quantizer is placed on the anchor scale the model is calibrated against.
    // A codec whose scale this engine declares none for leaves the number where it
    // stands, since there is no ratio to convert it by.
    const scale = cqMax(s.codec, engine, caps);
    const cq = scale > 0 ? (s.cq * ANCHOR_CQ_MAX) / scale : s.cq;
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
