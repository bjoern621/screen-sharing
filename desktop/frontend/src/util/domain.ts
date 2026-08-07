import { Option, Stream, TransportCarriage } from "../types/stream";
import { capabilities } from "../../wailsjs/go/models";

/**
 * The declarative domain model: one table per encoder family, video format,
 * chroma and rate-control mode, carrying every fact the UI derives from.
 * Constraints that must also hold in the encoder (which chroma a codec accepts,
 * whether the codec is implemented) are not here - those come from the backend
 * capability table (App.Capabilities), and which protocol carries a bitstream
 * comes from the backend transport table (App.TransportFormats), so a single
 * definition governs both sides. This file holds the presentation (label, tip,
 * link) and the bitrate heuristics, which are UI-only.
 *
 * A codec name like "hevc_vaapi" factors into a family (the backend, "vaapi")
 * and a format (the coding standard, "hevc"). The backend capability table
 * carries that factoring per codec; the two meta tables below supply the
 * presentation for each half, so the "Encoder" and "Codec" dropdowns are built
 * from small tables rather than one row per family×format combination.
 */

/** The backend's fixed codec facts: encoder family, allowed chromas, quantizer
 * scale and what the encoder cannot do. Which protocol carries the codec is a
 * fact about its bitstream rather than about the encoder, and lives in the
 * transport table instead (`carriesFormat`). */
export type Capability = capabilities.Codec;

/** One thing a codec cannot do, with the reason the form shows in place of the
 * option. `option` names the settings field it takes a value away from and `value`
 * which of that field's values, keyed as the settings themselves are, so a gap names
 * the control it greys. A gap with neither takes the codec off the engine itself. An
 * empty `engine` means the gap holds on both publish engines. */
export type Gap = capabilities.Gap;

/** The settings option a gap names. It is a settings field name because that is what
 * the backend's table keys its gaps by, so a gap and the control it greys are the
 * same identifier on both sides of the wire. */
export type GapOption = keyof Stream & string;

/** One decoder element and the pixel formats it decodes, from the backend decode
 * table (`App.Decoders`). It answers what a publish choice costs the viewer rather
 * than what this machine can do: a stream is published once and watched on whatever
 * hardware the watchers have, so a hardware verdict means some GPU decodes this. */
export type Decoder = capabilities.Decoder;

/** One audio codec the second track can be coded in, from the backend audio table
 * (`App.AudioCodecs`): the element each publish engine codes it with, the bitstream
 * format transports carry it under, and the rate and bitrate the track is coded at. */
export type AudioCodec = capabilities.AudioCodec;

/** The decoder families, as the backend names them. Anything but "software" runs on
 * fixed-function silicon. */
const DECODE_SOFTWARE = "software";

/** Vendor labels for the decoder families, so a sentence about hardware decoding
 * names the GPUs a user recognizes rather than the plugin the elements come from. */
const DECODE_FAMILY_LABEL: Record<string, string> = {
    va: "AMD and Intel on Linux (VA-API)",
    nvcodec: "NVIDIA (NVDEC)",
    qsv: "Intel (Quick Sync)",
    dxva: "any GPU on Windows (DXVA)",
};

/** The hardware decoders that take this format at this chroma, in table order. */
export function hardwareDecoders(
    decoders: Decoder[] | null,
    format: Format | undefined,
    chroma: Chroma
): Decoder[] {
    if (!decoders || !format) {
        return [];
    }
    return decoders.filter(
        d =>
            d.format === format &&
            d.family !== DECODE_SOFTWARE &&
            d.chromas.includes(chroma)
    );
}

/**
 * What decoding this stream costs a viewer, or "" while the decode table is
 * unresolved. It is a note and never a block: every format has a software decoder,
 * so the choice is between a viewer's GPU and a viewer's cores.
 *
 * Where some hardware decodes the pair, the sentence names those vendors, since which
 * ones they are is the whole point of the choice. Where none does, it carries one
 * family's reason, attributed to that family: the other families are out for reasons of
 * their own, and listing four of them in a tooltip states four times what the first
 * already shows.
 */
export function decodeNote(
    decoders: Decoder[] | null,
    codec: string,
    chroma: Chroma,
    caps: Capability[] | null
): string {
    const format = formatOf(codec, caps);
    if (!decoders || !format) {
        return "";
    }
    const hardware = hardwareDecoders(decoders, format, chroma);
    const vendors = [
        ...new Set(hardware.map(d => DECODE_FAMILY_LABEL[d.family] ?? d.family)),
    ];
    if (vendors.length > 0) {
        return `Viewers decode this in hardware on ${vendors.join(", ")}.`;
    }
    const software = decoders.find(
        d => d.format === format && d.family === DECODE_SOFTWARE
    );
    const blocked = decoders.find(
        d =>
            d.format === format &&
            d.family !== DECODE_SOFTWARE &&
            !d.chromas.includes(chroma)
    );
    const cost = software
        ? `every viewer decodes it on the CPU, through ${software.element}`
        : "every viewer decodes it on the CPU";
    if (!blocked) {
        return `No GPU decodes this pixel format, so ${cost}.`;
    }
    const vendor = DECODE_FAMILY_LABEL[blocked.family] ?? blocked.family;
    return `No GPU decodes this pixel format, so ${cost}. ${vendor}: ${blocked.reason}.`;
}

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

/**
 * Presentation for an encoder family (the "Encoder family" dropdown), plus the facts
 * that follow the backend rather than the codec or the mode. A field a family has no
 * property for is greyed whatever the rate-control mode, and the builders pin it off
 * rather than forwarding a value the encoder would ignore, so each such field is one
 * flag here instead of a family named in the rule that reads it.
 */
export interface FamilyMeta {
    label: string;
    tip: string;
    link?: string;
    /** Whether the encoders take the settings' B-frame count. */
    takesBframes?: boolean;
    /** Whether the encoders take the settings' encoder preset. */
    takesPreset?: boolean;
    /**
     * Whether the encoders come with a device rather than with a build. An absent
     * encoder is then the machine's answer (no such GPU, or no driver exposing that
     * encode entrypoint) where a software one's is the build's, which is what the
     * probe's reason says instead of "not detected" for a library nobody compiled in.
     */
    needsDevice?: boolean;
}

interface ChromaMeta {
    label: string;
    tip: string;
    link: string;
    /** Detail carried by the format, relative to 4:2:0 = 1.0. */
    weight: number;
    /** Raw bits per pixel per frame, for the lossless upper bound. */
    rawBpp: number;
    /** 4:2:0-family: chroma at quarter resolution. Subsampling and nothing else,
     * since p010le subsamples the same way and parts from yuv420p on bit depth. */
    is420: boolean;
    /** Bits per component. A decode profile pins depth and subsampling on
     * separate axes, so 4:2:0 carries no claim about depth. */
    bitDepth: number;
    /** RGB is inherently full range, so no color-range choice applies. */
    fullRange: boolean;
    /** What the format asks of a decoder beyond the 8-bit 4:2:0 profiles WHEP
     * negotiates; undefined where it asks nothing. */
    whepBlock?: string;
}

/** Whether a mode fixes the encoder preset, and to which ladder step. A mode that
 * pins carries the step in the same object, so the sentence naming it cannot be
 * written without one: an optional field would let the form claim a step the table
 * never declared. The value is the ffmpeg builder's nvencLivePreset, which is the
 * other copy of this fact. */
type PresetPinning =
    | { pinsPreset: false }
    | { pinsPreset: true; pinnedPreset: string };

type ModeMeta = {
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
    /** B-frames only help the lossy bitrate/quality modes, and only where the family
     * takes a count (FamilyMeta.takesBframes). */
    usesBframes: boolean;
} & PresetPinning;

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
export type Chroma = "gbrp" | "yuv444p" | "yuv422p" | "yuv420p" | "p010le";
export type Mode = "cbr" | "vbr" | "abr" | "crf" | "lossless";

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
        takesBframes: true,
        takesPreset: true,
        needsDevice: true,
    },
    vaapi: {
        label: "VAAPI (Intel / AMD)",
        link: VAAPI_LINK,
        tip: "Video Acceleration API: the shared Intel and AMD hardware encoder API on Linux, and the GPU option on a non-NVIDIA machine. Which formats a card encodes is the driver's answer and differs per GPU generation; every VAAPI encoder is 4:2:0 and none of them codes lossless.",
        needsDevice: true,
    },
    qsv: {
        label: "Intel Quick Sync (QSV)",
        link: QSV_LINK,
        tip: "Intel Quick Sync Video via oneVPL, the runtime Intel implements itself, where VAAPI is the vendor-neutral way to the same silicon. Intel GPUs only, and which formats one encodes differs per generation: VP9 encode arrives with Ice Lake and AV1 with Arc. Every QSV encoder is 4:2:0 and none codes lossless.",
        needsDevice: true,
    },
    amf: {
        label: "AMD AMF",
        link: AMF_LINK,
        tip: "Advanced Media Framework: AMD's own encoder API, driving the same silicon VAAPI reaches on an AMD card through AMD's closed-source runtime. VAAPI is the wider of the two, adding VP8 and VP9; AMF brings AMD's own rate control, whose peak-constrained VBR gives a burst ceiling a bitrate mode can target. Every AMF encoder is 4:2:0 and none codes lossless. x86_64 only, and the ffmpeg publish engine only.",
        needsDevice: true,
    },
    v4l2: {
        label: "V4L2 M2M (ARM SoC)",
        link: V4L2_LINK,
        tip: "Kernel memory-to-memory encoders on ARM SoCs (Raspberry Pi and similar).",
        needsDevice: true,
    },
    rkmpp: {
        label: "Rockchip MPP",
        link: "https://en.wikipedia.org/wiki/Rockchip",
        tip: "Rockchip Media Process Platform encoders (RK35xx-class SoCs).",
        needsDevice: true,
    },
    vulkan: {
        label: "Vulkan Video",
        link: VULKAN_LINK,
        tip: "The video-encode extensions a GPU driver implements itself, so one backend reaches NVIDIA, AMD and Intel silicon through the same API, and the only hardware family that is not tied to one vendor or one platform. On an AMD or Intel card it drives the same encoder block VAAPI does, through the vendor's Vulkan driver instead of Mesa. Which formats a driver implements the extension for differs per driver and GPU generation; every Vulkan encoder is 4:2:0 and none codes lossless. The ffmpeg publish engine only.",
        needsDevice: true,
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
        bitDepth: 8,
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
        bitDepth: 8,
        fullRange: false,
        whepBlock: "4:4:4 chroma",
    },
    yuv422p: {
        label: "yuv422p - Y′CbCr 4:2:2",
        link: "https://en.wikipedia.org/wiki/Chroma_subsampling",
        tip: "Chroma at half horizontal resolution and full vertical (4:2:2). Keeps the vertical colour detail 4:2:0 discards, for two thirds of 4:4:4's chroma samples: the middle ground between the two.",
        weight: 1.25,
        rawBpp: 16,
        is420: false,
        bitDepth: 8,
        fullRange: false,
        whepBlock: "4:2:2 chroma",
    },
    yuv420p: {
        label: "yuv420p - Y′CbCr 4:2:0",
        link: "https://en.wikipedia.org/wiki/Chroma_subsampling",
        tip: "Chroma at quarter resolution (4:2:0). Smallest and universally decodable - colored text/edges smear: the washed-out Discord/WebRTC look.",
        weight: 1.0,
        rawBpp: 12,
        is420: true,
        bitDepth: 8,
        fullRange: false,
    },
    p010le: {
        label: "p010le - 10-bit Y′CbCr 4:2:0",
        link: "https://en.wikipedia.org/wiki/Color_depth",
        tip: "10-bit Y′CbCr 4:2:0. More tonal resolution (HDR), still chroma-subsampled. Only useful for >8-bit sources.",
        weight: 1.2,
        rawBpp: 15,
        is420: true,
        bitDepth: 10,
        fullRange: false,
        whepBlock: "10-bit samples",
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
        tip: "VBR - constrained variable bitrate: targets the bitrate but bursts up to the max-bitrate ceiling on motion, holding quality where CBR would soften.\nNeeds headroom above the average. The ceiling is what separates it from ABR, so an encoder with no property for one does not offer this mode: the software encoders reach it through the ffmpeg publish engine and are greyed here on the GStreamer one, and the two AV1 encoders whose libraries take no ceiling at all are greyed on both.",
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
    // An encoder that cannot bound the burst at all has no VBR mode, which the
    // capability table declares as a mode gap. No rule here withholds the ceiling
    // field for that case: the mode carrying it is gone, so a rule would grey a
    // field under a mode that cannot be selected.
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
        knob: "vbvMs",
        families: ["nvenc"],
        forwards: false,
        reason: "the GStreamer nvcodec elements expose no rate-buffer property",
    },
    {
        engine: "gstreamer",
        knob: "vbvMs",
        families: ["qsv"],
        forwards: false,
        reason: "the GStreamer qsv elements expose no rate-buffer property, so the window binds on the ffmpeg publish engine only",
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
        families: ["vaapi", "amf", "vulkan", "qsv"],
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

/**
 * Presentation for a second-track capture source, keyed as the backend table names
 * it. Which sources exist and which of them this machine serves are the platform's
 * answers and are read off `App.AudioSources`, so this table carries what a reader
 * needs to choose and nothing the wire already states.
 *
 * Where the samples are read from is one of the things the wire states: it differs per
 * platform, so the row's own `server` is shown beside the label instead of a sentence
 * here naming one platform's mechanism to users on all three.
 */
export const AUDIO_META: Record<string, { label: string; tip: string; link?: string }> = {
    none: {
        label: "none - video only",
        tip: "No audio track: the stream carries video only.",
    },
    desktop: {
        label: "desktop - system audio",
        link: "https://wiki.archlinux.org/title/PulseAudio#Monitor_sources",
        tip: "Everything the machine plays, muxed in as a stereo second track. What codes that track is the audio codec field.",
    },
};

/**
 * Presentation for an audio codec, keyed as the backend table names it. The numbers
 * a codec is coded at (sample rate, bitrate) are the backend's and are read off
 * `App.AudioCodecs`, so this table carries what a reader needs to choose and nothing
 * the wire already states.
 */
export const AUDIO_CODEC_META: Record<string, { label: string; tip: string; link: string }> = {
    opus: {
        label: "opus - Opus",
        link: "https://en.wikipedia.org/wiki/Opus_(audio_format)",
        tip: "Opus: royalty-free, low-delay, and the only audio codec WebRTC negotiates. Its bitstream carries no sample rate a decoder has to match, so a track keeps playing whatever leg it is watched over.",
    },
    aac: {
        label: "aac - AAC",
        link: "https://en.wikipedia.org/wiki/Advanced_Audio_Coding",
        tip: "Advanced Audio Coding: what the FLV container carries and what a player opening an RTMP or HLS URL expects. Older and higher-delay than Opus, and no WebRTC leg negotiates it.",
    },
};

/** Whether this publish engine has an encoder element for the audio codec. An
 * engine absent from the row codes nothing, and the row's gap says why. */
export function audioCodesOn(a: AudioCodec, engine: Engine): boolean {
    return (a.encoders ?? []).some(e => e.engine === engine);
}

/**
 * The gap stating why this engine has no encoder for the audio codec, or undefined
 * where the table declares none. A gap naming no engine holds on both, as the video
 * gaps do.
 */
export function audioEngineGap(a: AudioCodec, engine: Engine): Gap | undefined {
    return (a.gaps ?? []).find(g => !g.engine || g.engine === engine);
}

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

/** One leg of a transport's carriage: the publisher-to-relay direction or the
 * relay-to-viewer one. The two are separate sets, since the relay serves
 * protocols it does not ingest and ingests formats it cannot serve back. */
export type Leg = "publish" | "watch";

/**
 * The backend's carriage table row for one transport, leg and engine, or undefined
 * where it has none.
 *
 * Carriage is per leg and per engine because the two axes each carry a fact: the
 * relay serves HLS and ingests nothing over it, and the two engines wrap different
 * muxers and source elements, so ffmpeg's WHIP muxer publishes H.264 alone where the
 * GStreamer one payloads what webrtcbin negotiates. A missing row is that statement:
 * the engine has no serialization for that leg of that protocol.
 *
 * On the publish leg the engine is the capture backend's publish engine. On the watch
 * leg "ffmpeg" is the URL-opening players and "gstreamer" is the native grid's
 * receiving pipeline; the browser is neither and carries its own table (webgrid.ts).
 */
function carriageRow(
    carriage: TransportCarriage[],
    transport: string,
    leg: Leg,
    engine: Engine
): TransportCarriage | undefined {
    return carriage.find(
        c => c.name === transport && c.leg === leg && c.engine === engine
    );
}

/**
 * The video formats a transport carries on one leg with one engine, or null where
 * the table states no such row. A null engine is the capture backend's before it
 * resolves, which names no row and so states nothing.
 */
export function legFormats(
    carriage: TransportCarriage[] | null,
    transport: string,
    leg: Leg,
    engine: Engine | null
): string[] | null {
    if (!carriage || !engine) {
        return null;
    }
    return carriageRow(carriage, transport, leg, engine)?.video ?? null;
}

/**
 * The audio codec formats a transport carries on one leg with one engine, or null
 * where the table states no such row. It reads the same rows as legFormats: a
 * protocol carries a video bitstream and an audio one over the same wire, and the
 * two sets differ, so RTMP takes AAC where WebRTC takes Opus.
 */
export function legAudioFormats(
    carriage: TransportCarriage[] | null,
    transport: string,
    leg: Leg,
    engine: Engine | null
): string[] | null {
    if (!carriage || !engine) {
        return null;
    }
    return carriageRow(carriage, transport, leg, engine)?.audio ?? null;
}

/**
 * Whether a transport carries a bitstream format on the given leg and engine.
 *
 * An unresolved table, engine or format states nothing, so a rule built on this
 * imposes no restriction before the facts arrive, as every other unresolved fact
 * here behaves. A resolved table with no matching row is the opposite: a transport
 * the table does not name carries nothing, and an engine with no row for the leg
 * cannot serialize it, so both report false rather than passing unnoticed.
 */
export function carriesFormat(
    carriage: TransportCarriage[] | null,
    transport: string,
    leg: Leg,
    engine: Engine | null,
    format: string | undefined
): boolean {
    if (!carriage || !engine || !format) {
        return true;
    }
    return legFormats(carriage, transport, leg, engine)?.includes(format) ?? false;
}

/** The transports carrying a format on the given leg with the given engine, for a
 * message that names where the combination would have worked. Empty while the table
 * or the engine is unresolved. */
export function carriersOf(
    carriage: TransportCarriage[] | null,
    leg: Leg,
    engine: Engine | null,
    format: string | undefined
): string[] {
    if (!carriage || !engine || !format) {
        return [];
    }
    return carriage
        .filter(
            c =>
                c.leg === leg &&
                c.engine === engine &&
                (c.video ?? []).includes(format)
        )
        .map(c => c.name);
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
 * The gap that keeps a codec from encoding with one value of one settings option on
 * the given engine, or undefined when that value reaches its encoder there. It is the
 * one gap lookup: the pixel format, the rate-control mode and the colour range are
 * asked for the same way, and so is any option the backend's table gains.
 *
 * A pixel format the codec cannot encode on either engine is absent from its
 * `chromas` instead.
 */
export function optionGapFor(
    codec: string,
    engine: Engine | null,
    option: GapOption,
    value: string,
    caps: Capability[] | null
): Gap | undefined {
    return gapsOn(codec, engine, caps).find(
        g => g.option === option && g.value === value
    );
}

/**
 * Every value of one settings option the codec cannot be encoded with on the given
 * engine, each mapped to the reason.
 *
 * An option whose values can also be unavailable for a reason that is not a gap has
 * its own walk over that option's value space (the chroma one names the formats the
 * codec does not list at all). This is for the options a gap is the only thing that
 * withholds, where the gaps are the whole list.
 */
export function optionGapsFor(
    codec: string,
    engine: Engine | null,
    option: GapOption,
    caps: Capability[] | null
): Record<string, string> {
    const out: Record<string, string> = {};
    for (const g of gapsOn(codec, engine, caps)) {
        if (g.option === option) {
            out[g.value] = g.reason;
        }
    }
    return out;
}

/**
 * The gap that takes a codec off the given engine altogether, or undefined when
 * that engine has an encoder for it. It is the gap that names no option, since no
 * value of any of them reaches an encoder that is not there. A codec gapped this way
 * is greyed in the codec dropdown while the other engine's capture backends still
 * offer it.
 */
export function engineGapFor(
    codec: string,
    engine: Engine | null,
    caps: Capability[] | null
): Gap | undefined {
    return gapsOn(codec, engine, caps).find(g => !g.option);
}

/** The encoder family of a codec, from the capability table (undefined until it loads). */
export function familyOf(codec: string, caps: Capability[] | null): Family | undefined {
    return findCapability(caps, codec)?.family as Family | undefined;
}

/**
 * The 51-point quantizer scale every figure stated codec-independently counts on:
 * the H.26x encoders' own. A number placed on it is converted to the running
 * codec's scale by scaleCq.
 */
export const ANCHOR_CQ_MAX = 51;

/**
 * Scale the codec's constant-quality knob counts on with the given engine, or 0
 * where the table declares none. It follows the encoder rather than the format: the
 * H.26x encoders reach 51, libvpx and the software AV1 ones 63, and an encoder taking
 * a raw quantizer index counts to 127 or 255. The same CQ number is therefore a
 * different quality per codec.
 *
 * The scale belongs to the property each engine sets rather than to the silicon
 * underneath, which is why it is asked per engine: ffmpeg's QSV encoders state a
 * quantizer on the H.26x scale where the qsv elements pass the format's own index
 * through.
 *
 * A zero is a scale the table does not declare, which is the case for the families
 * the argument builders do not map yet and while the engine or the table is
 * unresolved. It means the quantizer is not bounded here, not that it is bounded at
 * 51: pricing an unknown scale on the H.26x one would clamp a 255-point target to a
 * fifth of its range.
 */
export function cqMax(
    codec: string,
    engine: Engine | null,
    caps: Capability[] | null
): number {
    if (!engine) {
        return 0;
    }
    return findCapability(caps, codec)?.cqMax?.[engine] ?? 0;
}

/**
 * A quantizer target stated on the 51-point scale, placed on the codec's own. The
 * scales run to 51, 63, 127 and 255, so a bare number is a different quality per
 * codec and anything naming one has to say which scale it means. The quality
 * landmarks in the field's tooltip and the target a preset asks for are both stated
 * on the H.26x scale and converted here.
 *
 * A codec with no declared scale on this engine leaves the number where it was
 * stated, since there is no scale to place it on.
 */
export function scaleCq(
    cq51: number,
    codec: string,
    engine: Engine | null,
    caps: Capability[] | null
): number {
    const max = cqMax(codec, engine, caps);
    if (max <= 0) {
        return cq51;
    }
    return Math.round((cq51 * max) / ANCHOR_CQ_MAX);
}

/**
 * Highest bitrate target the codec's encoder accepts on the given engine, in Mbit/s,
 * or 0 when it takes any rate. An encoder with a ceiling refuses the encode rather
 * than clamping, so the field's own maximum comes from here and normalize repairs a
 * value carried over from a codec without one.
 */
export function bitrateLimit(
    codec: string,
    engine: Engine | null,
    caps: Capability[] | null
): number {
    if (!engine) {
        return 0;
    }
    return findCapability(caps, codec)?.bitrateLimitM?.[engine] ?? 0;
}

/** Human label for a codec, e.g. "HEVC / H.265 (VAAPI (Intel / AMD))". */
export function codecLabel(cap: Capability): string {
    const fmt = FORMAT_META[cap.format as Format]?.label ?? cap.format;
    const fam = FAMILY_META[cap.family as Family]?.label ?? cap.family;
    return `${fmt} (${fam})`;
}

/** Presentation and knob facts for a codec's family, undefined until the capability
 * table loads. */
export function familyMetaOf(
    codec: string,
    caps: Capability[] | null
): FamilyMeta | undefined {
    const family = familyOf(codec, caps);
    return family ? FAMILY_META[family] : undefined;
}

/**
 * Labels of the families whose meta sets the given flag, joined for a reason that
 * names who takes a field instead of stating one family by hand. A control greyed for
 * every other family says which ones own it, and the sentence follows the table when a
 * family gains the field.
 */
export function familiesWith(flag: (m: FamilyMeta) => boolean): string {
    return Object.values(FAMILY_META).filter(flag).map(m => m.label).join(", ");
}
