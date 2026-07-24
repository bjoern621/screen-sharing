import { Option } from "../types/stream";
import { capabilities } from "../../wailsjs/go/models";

/**
 * The declarative domain model: one table per codec, chroma and rate-control
 * mode, carrying every fact the UI derives from. Constraints that must also hold
 * in the encoder (which chroma a codec accepts, which transport carries it) are
 * not here - those come from the backend capability table (App.Capabilities), so
 * a single definition governs both sides. This file holds the presentation
 * (label, tip, link) and the bitrate/browser heuristics, which are UI-only.
 *
 * Deriving deps, normalization, the bitrate estimate and the browser verdict from
 * these tables removes the hand-kept duplication that let the disable rules and
 * the repair rules drift apart.
 */

/** The backend's fixed codec facts: nvenc flag, allowed chromas and transports. */
export type Capability = capabilities.Codec;

/** How widely a codec's 4:2:0 output decodes in browsers. */
type BrowserKind = "universal" | "modern" | "safari-only";

interface CodecMeta {
    label: string;
    tip: string;
    link: string;
    /** Relative coding efficiency: bits for equal quality, H.264 = 1.0. */
    efficiency: number;
    browser: BrowserKind;
}

interface ChromaMeta {
    label: string;
    tip: string;
    link: string;
    /** Detail carried by the format, relative to 4:2:0 = 1.0. */
    weight: number;
    /** Raw bits per pixel per frame, for the lossless upper bound. */
    rawBpp: number;
    /** 4:2:0-family (quarter-resolution chroma): the only browser-decodable kind. */
    is420: boolean;
    /** RGB is inherently full range, so no color-range choice applies. */
    fullRange: boolean;
    /** Why a non-4:2:0 format blocks browser playback; undefined for 4:2:0. */
    browserBlock?: string;
}

interface ModeMeta {
    label: string;
    tip: string;
    link: string;
    /** quality mode targets a constant quantizer (the CQ control). */
    usesCq: boolean;
    /** lossless output has no bitrate bound; the other modes do. */
    usesBitrate: boolean;
    /** B-frames only help lossy VBR, and only on NVENC. */
    usesBframes: boolean;
    /** latency mode pins the NVENC preset; pinnedPreset names the value. */
    pinsPreset: boolean;
    pinnedPreset?: string;
}

export type Codec = "hevc_nvenc" | "h264_nvenc" | "av1_nvenc" | "libx264";
export type Chroma = "gbrp" | "yuv444p" | "yuv420p" | "p010le";
export type Mode = "lossless" | "quality" | "latency";
export type AudioSource = "none" | "desktop";

const HEVC_LINK = "https://en.wikipedia.org/wiki/High_Efficiency_Video_Coding";
const AVC_LINK = "https://en.wikipedia.org/wiki/Advanced_Video_Coding";

export const CODEC_META: Record<Codec, CodecMeta> = {
    hevc_nvenc: {
        label: "HEVC / H.265 - NVENC hardware",
        link: HEVC_LINK,
        tip: "High Efficiency Video Coding (ITU-T H.265 | ISO/IEC 23008-2) on NVIDIA's encoder ASIC. Only NVENC codec with 4:4:4/RGB support here.",
        efficiency: 0.6,
        browser: "safari-only",
    },
    h264_nvenc: {
        label: "AVC / H.264 - NVENC hardware",
        link: AVC_LINK,
        tip: "Advanced Video Coding (ITU-T H.264 | ISO/IEC 14496-10) on NVIDIA's encoder ASIC. Widest decoder compatibility, less efficient than HEVC.",
        efficiency: 1.0,
        browser: "universal",
    },
    av1_nvenc: {
        label: "AV1 - NVENC hardware (4:2:0 only)",
        link: "https://en.wikipedia.org/wiki/AV1",
        tip: "AOMedia Video 1 on NVIDIA's encoder ASIC (RTX 40+). Most efficient per bit, but NVENC AV1 encodes 4:2:0 only.",
        efficiency: 0.5,
        browser: "modern",
    },
    libx264: {
        label: "AVC / H.264 - x264 software",
        link: "https://en.wikipedia.org/wiki/X264",
        tip: "AVC/H.264 in software (x264). CPU-heavy at high resolutions; fallback when no capable GPU encoder exists.",
        efficiency: 1.0,
        browser: "universal",
    },
};

export const CHROMA_META: Record<Chroma, ChromaMeta> = {
    gbrp: {
        label: "gbrp - planar RGB, no subsampling",
        link: "https://en.wikipedia.org/wiki/RGB_color_model",
        tip: "Planar RGB coded directly via HEVC Range Extensions (identity matrix). Zero color conversion - exact desktop pixels. Heaviest option.",
        weight: 2.0,
        rawBpp: 24,
        is420: false,
        fullRange: true,
        browserBlock: "RGB (HEVC Range Extensions)",
    },
    yuv444p: {
        label: "yuv444p - Y′CbCr 4:4:4",
        link: "https://en.wikipedia.org/wiki/Chroma_subsampling",
        tip: "Y′CbCr with full-resolution chroma (4:4:4). Near-indistinguishable from RGB after correct conversion; slightly cheaper than gbrp.",
        weight: 1.5,
        rawBpp: 24,
        is420: false,
        fullRange: false,
        browserBlock: "4:4:4 chroma",
    },
    yuv420p: {
        label: "yuv420p - Y′CbCr 4:2:0",
        link: "https://en.wikipedia.org/wiki/Chroma_subsampling",
        tip: "Chroma at quarter resolution (4:2:0). Smallest and universally decodable - colored text/edges smear: the washed-out Discord/WebRTC look.",
        weight: 1.0,
        rawBpp: 12,
        is420: true,
        fullRange: false,
    },
    p010le: {
        label: "p010le - 10-bit Y′CbCr 4:2:0",
        link: "https://en.wikipedia.org/wiki/Color_depth",
        tip: "10-bit Y′CbCr 4:2:0. More tonal resolution (HDR), still chroma-subsampled. Only useful for >8-bit sources.",
        weight: 1.2,
        rawBpp: 15,
        is420: true,
        fullRange: false,
    },
};

export const MODE_META: Record<Mode, ModeMeta> = {
    lossless: {
        label: "lossless - bit-exact",
        link: "https://en.wikipedia.org/wiki/Lossless_compression",
        tip: "Mathematically lossless: decoded output is bit-identical to input. Bitrate unbounded - bursts to hundreds of Mbit/s on motion. LAN only.",
        usesCq: false,
        usesBitrate: false,
        usesBframes: false,
        pinsPreset: false,
    },
    quality: {
        label: "quality - VBR + constant quantizer",
        link: "https://en.wikipedia.org/wiki/Variable_bitrate",
        tip: "Variable bitrate targeting a constant quantizer (CQ): quality held constant, bitrate varies with content. The bitrate bound only caps bursts.",
        usesCq: true,
        usesBitrate: true,
        usesBframes: true,
        pinsPreset: false,
    },
    latency: {
        label: "latency - CBR low-delay",
        link: "https://en.wikipedia.org/wiki/Constant_bitrate",
        tip: "Constant bitrate with low-delay tuning: fixed bandwidth, quality varies, smallest buffers and delay.",
        usesCq: false,
        usesBitrate: true,
        usesBframes: false,
        pinsPreset: true,
        pinnedPreset: "p5",
    },
};

export const AUDIO_META: Record<AudioSource, { label: string; tip: string; link?: string }> = {
    none: {
        label: "none - video only",
        tip: "No audio track: the stream carries video only.",
    },
    desktop: {
        label: "desktop - system audio",
        link: "https://wiki.archlinux.org/title/PulseAudio#Monitor_sources",
        tip: "Everything the machine plays, captured from the default output's monitor source (PulseAudio/PipeWire) and muxed in as 128 kbit/s stereo Opus.",
    },
};

/** Builds the SelectField option list for a meta table, preserving key order. */
export function metaOptions<K extends string>(
    meta: Record<K, { label: string; tip: string; link?: string }>
): Option[] {
    return Object.entries(meta).map(([value, m]) => {
        const o = m as { label: string; tip: string; link?: string };
        return { value, label: o.label, tip: o.tip, link: o.link };
    });
}

/** Fallback codec when the chosen one is unavailable: software, always present. */
export const FALLBACK_CODEC: Codec = "libx264";

/** Looks up a codec's backend capability facts in the fetched table. */
export function findCapability(
    caps: Capability[] | null,
    codec: string
): Capability | undefined {
    return caps?.find(c => c.name === codec);
}

/**
 * Whether codec runs on NVENC. Prefers the fetched capability fact; before it
 * resolves, falls back to the "_nvenc" name suffix so preset/B-frame controls do
 * not flicker during startup.
 */
export function isNvenc(codec: string, caps: Capability[] | null): boolean {
    const cap = findCapability(caps, codec);
    return cap ? cap.nvenc : codec.endsWith("_nvenc");
}
