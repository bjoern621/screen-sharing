import { Option } from "../types/stream";
import { capabilities } from "../../wailsjs/go/models";

/**
 * The declarative domain model: one table per encoder family, video format,
 * chroma and rate-control mode, carrying every fact the UI derives from.
 * Constraints that must also hold in the encoder (which chroma a codec accepts,
 * which transport carries it, whether the codec is implemented) are not here -
 * those come from the backend capability table (App.Capabilities), so a single
 * definition governs both sides. This file holds the presentation (label, tip,
 * link) and the bitrate/browser heuristics, which are UI-only.
 *
 * A codec name like "hevc_vaapi" factors into a family (the backend, "vaapi")
 * and a format (the coding standard, "hevc"). The backend capability table
 * carries that factoring per codec; the two meta tables below supply the
 * presentation for each half, so the "Encoder" and "Codec" dropdowns are built
 * from small tables rather than one row per family×format combination.
 */

/** The backend's fixed codec facts: nvenc flag, allowed chromas and transports. */
export type Capability = capabilities.Codec;

/** How widely a format's 4:2:0 output decodes in browsers. */
type BrowserKind = "universal" | "modern" | "safari-only";

/** Presentation and heuristics for a video coding format, independent of the
 * encoder backend that produces it. Coding efficiency and browser decodability
 * follow the format (H.264 vs HEVC vs AV1), not the family (nvenc vs vaapi). */
interface FormatMeta {
    label: string;
    tip: string;
    link: string;
    /** Relative coding efficiency: bits for equal quality, H.264 = 1.0. */
    efficiency: number;
    browser: BrowserKind;
}

/** Presentation for an encoder backend (the "Encoder" dropdown). */
interface FamilyMeta {
    label: string;
    tip: string;
    link?: string;
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
    /** crf mode targets a constant quantizer (the CQ control). */
    usesCq: boolean;
    /** cbr/vbr/abr target a bitrate; crf and lossless do not. */
    usesBitrate: boolean;
    /** vbr sets a burst ceiling (max bitrate) above the target. */
    usesMaxrate: boolean;
    /** cbr/vbr bound the rate with a VBV buffer whose size is tunable. */
    usesVbv: boolean;
    /** B-frames only help the lossy bitrate/quality modes, and only on NVENC. */
    usesBframes: boolean;
    /** cbr pins the NVENC preset to the low-latency value; pinnedPreset names it. */
    pinsPreset: boolean;
    pinnedPreset?: string;
}

/** Encoder backends. A codec name factors into a family and a format; the
 * backend capability table (capabilities.Codec) carries the factoring. */
export type Family =
    "software" | "nvenc" | "vaapi" | "qsv" | "amf" | "v4l2" | "rkmpp" | "vulkan";
/** Video coding formats, independent of the encoder backend. */
export type Format = "h264" | "hevc" | "av1" | "vp9" | "vp8";
export type Chroma = "gbrp" | "yuv444p" | "yuv420p" | "p010le";
export type Mode = "cbr" | "vbr" | "abr" | "crf" | "lossless";
export type AudioSource = "none" | "desktop";

const HEVC_LINK = "https://en.wikipedia.org/wiki/High_Efficiency_Video_Coding";
const AVC_LINK = "https://en.wikipedia.org/wiki/Advanced_Video_Coding";
const AV1_LINK = "https://en.wikipedia.org/wiki/AV1";
const VP9_LINK = "https://en.wikipedia.org/wiki/VP9";
const VP8_LINK = "https://en.wikipedia.org/wiki/VP8";
const NVENC_LINK = "https://en.wikipedia.org/wiki/Nvidia_NVENC";
const VAAPI_LINK = "https://en.wikipedia.org/wiki/Video_Acceleration_API";
const QSV_LINK = "https://en.wikipedia.org/wiki/Intel_Quick_Sync_Video";
const AMF_LINK = "https://en.wikipedia.org/wiki/Video_Coding_Engine";
const V4L2_LINK = "https://en.wikipedia.org/wiki/Video4Linux";
const VULKAN_LINK = "https://en.wikipedia.org/wiki/Vulkan";

export const FORMAT_META: Record<Format, FormatMeta> = {
    h264: {
        label: "AVC / H.264",
        link: AVC_LINK,
        tip: "Advanced Video Coding (ITU-T H.264 | ISO/IEC 14496-10). Widest decoder compatibility, least efficient of the modern formats.",
        efficiency: 1.0,
        browser: "universal",
    },
    hevc: {
        label: "HEVC / H.265",
        link: HEVC_LINK,
        tip: "High Efficiency Video Coding (ITU-T H.265 | ISO/IEC 23008-2). Around 40% smaller than H.264 at equal quality; browser playback is limited to Safari or an OS extension.",
        efficiency: 0.6,
        browser: "safari-only",
    },
    av1: {
        label: "AV1",
        link: AV1_LINK,
        tip: "AOMedia Video 1. Most efficient per bit. Hardware encoders are recent: NVENC on RTX 40+, Intel Arc, AMD RDNA3+.",
        efficiency: 0.5,
        browser: "modern",
    },
    vp9: {
        label: "VP9",
        link: VP9_LINK,
        tip: "Google VP9. Royalty-free, decodes in most non-Safari browsers; efficiency sits between H.264 and HEVC.",
        efficiency: 0.6,
        browser: "modern",
    },
    vp8: {
        label: "VP8",
        link: VP8_LINK,
        tip: "Google VP8. Older royalty-free format with broad WebM/WebRTC support; efficiency near H.264.",
        efficiency: 1.0,
        browser: "modern",
    },
};

export const FAMILY_META: Record<Family, FamilyMeta> = {
    software: {
        label: "Software (x264)",
        link: "https://en.wikipedia.org/wiki/X264",
        tip: "CPU encoding via x264. Always available, no GPU needed; CPU-heavy at high resolution and frame rate.",
    },
    nvenc: {
        label: "NVIDIA NVENC",
        link: NVENC_LINK,
        tip: "NVIDIA's dedicated encoder ASIC. Needs an NVIDIA GPU, its driver, and an nvenc-enabled ffmpeg.",
    },
    vaapi: {
        label: "VAAPI (Intel / AMD)",
        link: VAAPI_LINK,
        tip: "Video Acceleration API: the shared Intel + AMD hardware encoder path on Linux. The single most useful backend on a non-NVIDIA desktop.",
    },
    qsv: {
        label: "Intel Quick Sync (QSV)",
        link: QSV_LINK,
        tip: "Intel Quick Sync Video via oneVPL. Intel GPUs only; often better quality and rate control than generic VAAPI on the same silicon.",
    },
    amf: {
        label: "AMD AMF",
        link: AMF_LINK,
        tip: "AMD Media Framework. AMD GPUs; on Linux, VAAPI is usually the stronger path for the same cards.",
    },
    v4l2: {
        label: "V4L2 M2M (ARM SoC)",
        link: V4L2_LINK,
        tip: "Kernel memory-to-memory encoders on ARM SoCs (Raspberry Pi and similar).",
    },
    rkmpp: {
        label: "Rockchip MPP",
        link: "https://en.wikipedia.org/wiki/Rockchip",
        tip: "Rockchip Media Process Platform encoders (RK35xx-class SoCs).",
    },
    vulkan: {
        label: "Vulkan Video",
        link: VULKAN_LINK,
        tip: "Cross-vendor hardware encoding through the Vulkan video-encode extensions. Newest and least mature path.",
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
    cbr: {
        label: "CBR - constant bitrate",
        link: "https://en.wikipedia.org/wiki/Constant_bitrate",
        tip: "CBR - constant bitrate: the encoder holds the target every second, so bandwidth is fixed and quality floats with the scene.\nLow-delay tuning keeps buffers smallest. Best when the link has a hard bandwidth cap.",
        usesCq: false,
        usesBitrate: true,
        usesMaxrate: false,
        usesVbv: true,
        usesBframes: false,
        pinsPreset: true,
        pinnedPreset: "p5",
    },
    vbr: {
        label: "VBR - constrained (target + ceiling)",
        link: "https://en.wikipedia.org/wiki/Variable_bitrate",
        tip: "VBR - constrained variable bitrate: targets the bitrate but bursts up to the max-bitrate ceiling on motion, holding quality where CBR would soften.\nNeeds headroom above the average. The ceiling binds on NVENC and the ffmpeg x264 path; x264 over the portal backend runs it as uncapped ABR.",
        usesCq: false,
        usesBitrate: true,
        usesMaxrate: true,
        usesVbv: true,
        usesBframes: true,
        pinsPreset: false,
    },
    abr: {
        label: "ABR - average bitrate",
        link: "https://en.wikipedia.org/wiki/Variable_bitrate",
        tip: "ABR - average bitrate: one pass toward the target average with no ceiling, so hard frames burst freely and quality holds.\nSimplest bitrate mode; fits an unmetered LAN where bursts are fine.",
        usesCq: false,
        usesBitrate: true,
        usesMaxrate: false,
        usesVbv: false,
        usesBframes: true,
        pinsPreset: false,
    },
    crf: {
        label: "CRF / CQ - constant quality",
        link: "https://en.wikipedia.org/wiki/Variable_bitrate",
        tip: "CRF / constant-QP - constant quality: the encoder spends whatever bitrate holds the quantizer target (CQ) steady.\nBitrate rises on motion and falls on a static screen; quality-first with no rate bound.",
        usesCq: true,
        usesBitrate: false,
        usesMaxrate: false,
        usesVbv: false,
        usesBframes: true,
        pinsPreset: false,
    },
    lossless: {
        label: "Lossless - QP 0, bit-exact",
        link: "https://en.wikipedia.org/wiki/Lossless_compression",
        tip: "QP 0 - no quantization: the encoder discards no detail, so decoded output is bit-identical to the source.\nNo rate control, so bitrate bursts to hundreds of Mbit/s on motion. LAN only.",
        usesCq: false,
        usesBitrate: false,
        usesMaxrate: false,
        usesVbv: false,
        usesBframes: false,
        pinsPreset: false,
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
export const FALLBACK_CODEC = "libx264";

/** Looks up a codec's backend capability facts in the fetched table. */
export function findCapability(
    caps: Capability[] | null,
    codec: string
): Capability | undefined {
    return caps?.find(c => c.name === codec);
}

/** The video format of a codec, from the capability table (undefined until it loads). */
export function formatOf(codec: string, caps: Capability[] | null): Format | undefined {
    return findCapability(caps, codec)?.format as Format | undefined;
}

/** The encoder family of a codec, from the capability table (undefined until it loads). */
export function familyOf(codec: string, caps: Capability[] | null): Family | undefined {
    return findCapability(caps, codec)?.family as Family | undefined;
}

/** Human label for a codec, e.g. "HEVC / H.265 (VAAPI (Intel / AMD))". */
export function codecLabel(cap: Capability): string {
    const fmt = FORMAT_META[cap.format as Format]?.label ?? cap.format;
    const fam = FAMILY_META[cap.family as Family]?.label ?? cap.family;
    return `${fmt} (${fam})`;
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
