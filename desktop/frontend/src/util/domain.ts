import { Option } from "../types/stream";
import { capabilities } from "../../wailsjs/go/models";

/**
 * The declarative domain model: one table per encoder family, video format,
 * chroma and rate-control mode, carrying every fact the UI derives from.
 * Constraints that must also hold in the encoder (which chroma a codec accepts,
 * which transport publishes it, whether the codec is implemented) are not here -
 * those come from the backend capability table (App.Capabilities), so a single
 * definition governs both sides. This file holds the presentation (label, tip,
 * link) and the bitrate heuristics, which are UI-only.
 *
 * A codec name like "hevc_vaapi" factors into a family (the backend, "vaapi")
 * and a format (the coding standard, "hevc"). The backend capability table
 * carries that factoring per codec; the two meta tables below supply the
 * presentation for each half, so the "Encoder" and "Codec" dropdowns are built
 * from small tables rather than one row per family×format combination.
 */

/** The backend's fixed codec facts: nvenc flag, allowed chromas, and the
 * transports that can publish the codec. `transports` is the publish leg
 * (publisher to relay); which transport a viewer receives over is chosen per
 * viewer and is not in this table. */
export type Capability = capabilities.Codec;

/** One thing a codec cannot do, with the reason the form shows in place of the
 * option. The field it sets names the axis: `chroma` for a pixel format the
 * engine's encoder will not take, `mode` for a rate-control mode it has no form
 * of, neither for the codec itself. An empty `engine` means the gap holds on both
 * publish engines. */
export type Gap = capabilities.Gap;

/** Presentation and heuristics for a video coding format, independent of the
 * encoder family that produces it. Coding efficiency follows the format (H.264
 * vs HEVC vs AV1), not the family (nvenc vs vaapi). */
interface FormatMeta {
    label: string;
    tip: string;
    link: string;
    /** Relative coding efficiency: bits for equal quality, H.264 = 1.0. */
    efficiency: number;
}

/** Presentation for an encoder family (the "Encoder family" dropdown). */
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
    /** 4:2:0-family (quarter-resolution chroma): the only kind WebRTC negotiates. */
    is420: boolean;
    /** RGB is inherently full range, so no color-range choice applies. */
    fullRange: boolean;
    /** What a non-4:2:0 format asks of a decoder that WHEP will not negotiate;
     * undefined for 4:2:0. */
    whepBlock?: string;
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

/** The publish engine that runs a capture backend, from App.CaptureEngines. The
 * screen grabbers build one ffmpeg command; the portal capture backend builds a
 * GStreamer pipeline. Which codecs, pixel formats and rate-control knobs reach the
 * encoder differs between them, so the settings form needs to know which engine the
 * selected capture backend uses. */
export type Engine = "ffmpeg" | "gstreamer";

/** Display name of a publish engine, for a label or a sentence shown to the user.
 * The two spellings are the projects' own, and the UI uses no other name for them. */
export const ENGINE_LABEL: Record<Engine, string> = {
    ffmpeg: "ffmpeg",
    gstreamer: "GStreamer",
};

/**
 * The publish engine that runs a capture backend, or null while the map from the
 * backend is unresolved. The engine is never chosen directly: picking the capture
 * backend fixes it, which is why every rule keyed by engine is presented to the user
 * as a property of the capture backend.
 */
export function engineFor(
    capture: string,
    captureEngines: Record<string, string> | null
): Engine | null {
    const name = captureEngines?.[capture];
    return name === "ffmpeg" || name === "gstreamer" ? name : null;
}

/** One rate-control control on the settings form, named after the settings field
 * it edits. */
export type Knob =
    "cq" | "bitrateM" | "maxrateM" | "vbvMs" | "bframes" | "encPreset" | "gop";

/** Encoder backends. A codec name factors into a family and a format; the
 * backend capability table (capabilities.Codec) carries the factoring. */
export type Family =
    "software" | "nvenc" | "vaapi" | "qsv" | "amf" | "v4l2" | "rkmpp" | "vulkan";
/** Video coding formats, independent of the encoder family. */
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
    },
    hevc: {
        label: "HEVC / H.265",
        link: HEVC_LINK,
        tip: "High Efficiency Video Coding (ITU-T H.265 | ISO/IEC 23008-2). Around 40% smaller than H.264 at equal quality; browser playback is limited to Safari or an OS extension.",
        efficiency: 0.6,
    },
    av1: {
        label: "AV1",
        link: AV1_LINK,
        tip: "AOMedia Video 1. Most efficient per bit. Hardware encoders are recent: NVENC on RTX 40+, Intel Arc, AMD RDNA3+.",
        efficiency: 0.5,
    },
    vp9: {
        label: "VP9",
        link: VP9_LINK,
        tip: "Google VP9. Royalty-free, decodes in most non-Safari browsers; efficiency sits between H.264 and HEVC.",
        efficiency: 0.6,
    },
    vp8: {
        label: "VP8",
        link: VP8_LINK,
        tip: "Google VP8. Older royalty-free format with broad WebM/WebRTC support; efficiency near H.264.",
        efficiency: 1.0,
    },
};

export const FAMILY_META: Record<Family, FamilyMeta> = {
    software: {
        label: "Software (CPU)",
        link: "https://en.wikipedia.org/wiki/X264",
        tip: "CPU encoding, one encoder per format: x264, x265, libvpx for VP8 and VP9, and three AV1 encoders that trade speed against reach. No GPU needed; CPU-heavy at high resolution and frame rate, more so the newer the format.",
    },
    nvenc: {
        label: "NVIDIA NVENC",
        link: NVENC_LINK,
        tip: "NVIDIA's dedicated encoder ASIC. Needs an NVIDIA GPU with its driver, and NVENC support in the publish engine: an nvenc-enabled ffmpeg, or the GStreamer nvcodec elements.",
    },
    vaapi: {
        label: "VAAPI (Intel / AMD)",
        link: VAAPI_LINK,
        tip: "Video Acceleration API: the shared Intel and AMD hardware encoder API on Linux, and the GPU option on a non-NVIDIA machine. Which formats a card encodes is the driver's answer and differs per GPU generation; every VAAPI encoder is 4:2:0 and none of them codes lossless.",
    },
    qsv: {
        label: "Intel Quick Sync (QSV)",
        link: QSV_LINK,
        tip: "Intel Quick Sync Video via oneVPL. Intel GPUs only; often better quality and rate control than generic VAAPI on the same silicon.",
    },
    amf: {
        label: "AMD AMF",
        link: AMF_LINK,
        tip: "Advanced Media Framework: AMD's own encoder API, driving the same silicon VAAPI reaches on an AMD card through AMD's closed-source runtime. VAAPI is the wider of the two, adding VP8 and VP9; AMF brings AMD's own rate control, whose peak-constrained VBR gives a burst ceiling a bitrate mode can target. Every AMF encoder is 4:2:0 and none codes lossless. x86_64 only, and the ffmpeg publish engine only.",
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
        tip: "Cross-vendor hardware encoding through the Vulkan video-encode extensions. Newest and least mature of the hardware families.",
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
        whepBlock: "RGB (HEVC Range Extensions)",
    },
    yuv444p: {
        label: "yuv444p - Y′CbCr 4:4:4",
        link: "https://en.wikipedia.org/wiki/Chroma_subsampling",
        tip: "Y′CbCr with full-resolution chroma (4:4:4). Near-indistinguishable from RGB after correct conversion; slightly cheaper than gbrp.",
        weight: 1.5,
        rawBpp: 24,
        is420: false,
        fullRange: false,
        whepBlock: "4:4:4 chroma",
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
        tip: "VBR - constrained variable bitrate: targets the bitrate but bursts up to the max-bitrate ceiling on motion, holding quality where CBR would soften.\nNeeds headroom above the average. The ceiling binds on NVENC and on the software encoders through the ffmpeg publish engine; on the GStreamer publish engine they run it as uncapped ABR.",
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

/**
 * One place where a publish engine's encoder builder departs from the mode
 * table: a knob the mode uses that the engine ignores, or one the engine
 * forwards in a mode the table marks unused.
 *
 * MODE_META says which knobs a rate-control concept needs. Whether the value
 * reaches the encoder also depends on which engine builds the command, because
 * the two express the same modes through different properties: the GStreamer
 * elements have no NVENC preset ladder, x264enc cannot raise a ceiling above its
 * bitrate, and vpxenc has no unbounded constant-quality mode. Both facts decide
 * whether a field is live, so a knob the engine drops is greyed with the engine's
 * reason instead of looking effective.
 *
 * A knob an encoder library has no form of at all is a rule with no `engine`,
 * since both builders hit the same wall: SVT-AV1 refuses a rate ceiling outside
 * constant-quality mode and rav1e has no ceiling or rate buffer whatsoever.
 *
 * The rules mirror the two builders, `encoderArgs` in ffmpeg/encoders.go and
 * `gstEncoder` in publish/gstencoders.go. A missing engine matches both; empty
 * codecs and families match every codec the engine builds; empty modes match
 * every mode.
 */
interface EngineRule {
    engine?: Engine;
    knob: Knob;
    /** Codec names the rule covers, unioned with families. */
    codecs?: string[];
    /** Encoder families the rule covers, unioned with codecs. */
    families?: Family[];
    /** Modes the rule covers. */
    modes?: Mode[];
    /** true: the builder forwards the value even where the mode table marks the
     * knob unused. false: it ignores a value the mode would use. */
    forwards: boolean;
    /** An ignored knob's reason states why the value never reaches the encoder; a
     * forwarded knob's states what it does there. Both are shown to the user. */
    reason: string;
}

const ENGINE_RULES: EngineRule[] = [
    {
        engine: "gstreamer",
        knob: "encPreset",
        forwards: false,
        reason: "no GStreamer encoder element exposes the p1-p7 preset ladder, so it is reachable on the ffmpeg publish engine only",
    },
    // The two AV1 encoders whose libraries have no ceiling or rate buffer at all
    // come first: their reason names the library, which beats the engine-wide one
    // below it.
    {
        knob: "maxrateM",
        codecs: ["libsvtav1"],
        modes: ["vbr"],
        forwards: false,
        reason: "SVT-AV1 accepts a rate ceiling in constant-quality mode only and rejects the encode outright in VBR, so constrained VBR runs as uncapped ABR",
    },
    {
        knob: "maxrateM",
        codecs: ["librav1e"],
        modes: ["vbr"],
        forwards: false,
        reason: "rav1e's one-pass rate control takes a bitrate target and nothing above it, so constrained VBR runs as uncapped ABR",
    },
    {
        knob: "vbvMs",
        codecs: ["librav1e"],
        forwards: false,
        reason: "rav1e sizes no rate buffer, in any mode",
    },
    {
        engine: "gstreamer",
        knob: "gop",
        codecs: ["librav1e"],
        forwards: false,
        reason: "the rav1enc element exposes no keyframe-interval property, so the GStreamer publish engine leaves rav1e's own default standing",
    },
    {
        engine: "gstreamer",
        knob: "maxrateM",
        families: ["software"],
        modes: ["vbr"],
        forwards: false,
        reason: "the GStreamer software encoder elements take a bitrate target and no ceiling above it, so constrained VBR runs as uncapped ABR",
    },
    {
        engine: "gstreamer",
        knob: "vbvMs",
        modes: ["vbr"],
        forwards: false,
        reason: "the GStreamer encoder elements size their rate buffer in CBR only",
    },
    {
        engine: "gstreamer",
        knob: "vbvMs",
        families: ["nvenc"],
        forwards: false,
        reason: "the GStreamer nvcodec elements expose no rate-buffer property",
    },
    {
        engine: "ffmpeg",
        knob: "bitrateM",
        families: ["nvenc"],
        modes: ["crf"],
        forwards: true,
        reason: "NVENC constant quality caps its bursts at this bitrate; the quantizer target still drives the look.",
    },
    {
        engine: "gstreamer",
        knob: "bitrateM",
        codecs: ["libvpx-vp9", "libvpx"],
        modes: ["crf"],
        forwards: true,
        reason: "the vp8enc and vp9enc elements have no unbounded constant-quality mode, so on the GStreamer publish engine this bitrate is the cap their CQ rate control stays under.",
    },
    {
        knob: "bitrateM",
        families: ["vaapi", "amf"],
        modes: ["abr"],
        forwards: true,
        reason: "the fixed-function encoders always code against a rate ceiling, so this target is sent with twice itself as one; the average is what the target holds.",
    },
    {
        knob: "bframes",
        families: ["amf"],
        forwards: false,
        reason: "AMD's HEVC encoder codes no B-frames at all, and its H.264 and AV1 ones are driven with the B-picture pattern switched off, so a live stream pays none of their reorder delay",
    },
];

/**
 * The engine rule that governs a knob for the given engine, codec and mode, or
 * undefined when the builder treats the knob exactly as the mode table says.
 * Earlier rules win, so a mode-specific reason precedes a codec-wide one.
 *
 * A rule naming no engine describes the encoder library rather than a builder, so
 * it holds while the capture's engine is still unresolved; an engine-specific one
 * does not apply until the engine is known.
 */
export function findEngineRule(
    knob: Knob,
    engine: Engine | null,
    codec: string,
    mode: Mode,
    caps: Capability[] | null
): EngineRule | undefined {
    const cap = findCapability(caps, codec);
    return ENGINE_RULES.find(r => {
        if ((r.engine && r.engine !== engine) || r.knob !== knob) {
            return false;
        }
        if (r.modes && !r.modes.includes(mode)) {
            return false;
        }
        const selects =
            (r.codecs?.includes(codec) ?? false) ||
            (!!cap && (r.families?.includes(cap.family as Family) ?? false));
        return selects || (!r.codecs && !r.families);
    });
}

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

/**
 * What one encoder implementation adds beyond its format, appended to the format's
 * own tooltip in the codec dropdown. Defined only where the format does not
 * identify the encoder: three software encoders produce AV1, and they differ in
 * speed, chroma reach and which rate-control knobs they honor. Everything else in
 * this file is keyed by family or format for that reason, so this table stays as
 * small as the collisions it explains.
 */
export const ENCODER_TIPS: Record<string, string> = {
    "libaom-av1":
        "libaom, the AV1 reference encoder: the only software AV1 here that codes 4:4:4 and RGB, and the slowest of the three even in its realtime mode.",
    libsvtav1:
        "SVT-AV1: the fastest realtime AV1, which is what makes the format usable at desktop resolutions. 4:2:0 and 10-bit only.",
    librav1e:
        "rav1e: 4:4:4 and 10-bit AV1 between the other two in speed. One bitrate target with no ceiling and no rate buffer, and its quantizer counts to 255.",
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

/**
 * The gaps a codec carries that bind on the given engine. A gap carrying no engine
 * holds on both, so it applies while the capture's engine is unresolved; an
 * engine-specific one waits for the engine, matching how the backend's Validate
 * reads the same rows.
 */
function gapsOn(
    codec: string,
    engine: Engine | null,
    caps: Capability[] | null
): Gap[] {
    return (findCapability(caps, codec)?.gaps ?? []).filter(
        g => !g.engine || g.engine === engine
    );
}

/**
 * The gap that keeps a codec out of a rate-control mode on the given engine, or
 * undefined when the mode reaches its encoder.
 */
export function modeGapFor(
    codec: string,
    engine: Engine | null,
    mode: Mode,
    caps: Capability[] | null
): Gap | undefined {
    return gapsOn(codec, engine, caps).find(g => g.mode === mode && !g.chroma);
}

/**
 * The gap that keeps a codec from encoding a pixel format on the given engine, or
 * undefined when the format reaches its encoder there. A format the codec cannot
 * encode on either engine is absent from its `chromas` instead.
 */
export function chromaGapFor(
    codec: string,
    engine: Engine | null,
    chroma: Chroma,
    caps: Capability[] | null
): Gap | undefined {
    return gapsOn(codec, engine, caps).find(g => g.chroma === chroma && !g.mode);
}

/**
 * The gap that takes a codec off the given engine altogether, or undefined when
 * that engine has an encoder for it. A codec gapped this way is greyed in the codec
 * dropdown while the other engine's capture backends still offer it.
 */
export function engineGapFor(
    codec: string,
    engine: Engine | null,
    caps: Capability[] | null
): Gap | undefined {
    return gapsOn(codec, engine, caps).find(g => !g.chroma && !g.mode);
}

/** The encoder family of a codec, from the capability table (undefined until it loads). */
export function familyOf(codec: string, caps: Capability[] | null): Family | undefined {
    return findCapability(caps, codec)?.family as Family | undefined;
}

/**
 * Scale the codec's constant-quality knob counts on. It follows the encoder rather
 * than the format: the H.26x encoders reach 51, libvpx and the software AV1 ones 63,
 * and an encoder taking a raw quantizer index counts to 127 or 255. The same CQ
 * number is therefore a different quality per codec. Falls back to the 51-point
 * scale while the capability table is unresolved or carries no figure for the codec.
 */
export function cqMax(codec: string, caps: Capability[] | null): number {
    return findCapability(caps, codec)?.cqMax || 51;
}

/**
 * Highest bitrate target the codec's encoder accepts, in Mbit/s, or 0 when it takes
 * any rate. Only one encoder has a ceiling, and it refuses the encode rather than
 * clamping, so the field's own maximum comes from here and normalize repairs a value
 * carried over from a codec without one.
 */
export function bitrateLimit(codec: string, caps: Capability[] | null): number {
    return findCapability(caps, codec)?.bitrateLimitM ?? 0;
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
