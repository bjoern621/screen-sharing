import { Deps, EncoderInfo, PlatformInfo, Stream } from "../types/stream";
import {
    Capability, CHROMA_META, CODEC_META, Codec, Chroma, FALLBACK_CODEC,
    MODE_META, Mode, findCapability, isNvenc,
} from "./domain";

// Fallback chroma preference, highest quality first. normalize walks this order
// and picks the first format the repaired codec supports, so gbrp (only hevc)
// drops to yuv444p and AV1's rejected 4:4:4 drops to yuv420p.
const CHROMA_FALLBACK_ORDER: Chroma[] = [
    "yuv444p", "yuv420p", "p010le", "gbrp",
];

/**
 * Capture APIs that cannot run on the given platform, each mapped to the reason.
 * A null platform (not yet detected) imposes no restriction.
 */
function unavailableCaptures(
    platform: PlatformInfo | null
): Record<string, string> {
    if (!platform) {
        return {};
    }
    if (platform.os === "windows") {
        return {
            x11grab: "X11 capture is Linux-only",
            kmsgrab: "DRM/KMS capture is Linux-only",
            portal: "PipeWire ScreenCast is Linux-only",
        };
    }
    if (platform.os === "linux") {
        const out: Record<string, string> = {
            ddagrab: "DXGI Desktop Duplication is Windows-only",
            gdigrab: "GDI capture is Windows-only",
        };
        if (platform.display === "wayland") {
            out.x11grab =
                "Wayland session: x11grab only sees XWayland windows, not the Wayland desktop - use portal";
        } else {
            out.portal = "PipeWire ScreenCast needs a Wayland session";
        }
        return out;
    }
    return {};
}

/**
 * Codecs whose test encode failed on this machine, each mapped to the reason.
 * A null encoder set (probe not yet resolved) imposes no restriction, and codecs
 * the backend never probes (libx264) never appear here.
 */
function unavailableCodecs(
    encoders: EncoderInfo | null
): Record<string, string> {
    if (!encoders) {
        return {};
    }
    const out: Record<string, string> = {};
    for (const [codec, ok] of Object.entries(encoders.usable)) {
        if (!ok) {
            out[codec] = "no NVIDIA encoder detected on this machine";
        }
    }
    return out;
}

/** Whether codec can run here. Unknown probe or unprobed codec counts as usable. */
function codecUsable(codec: string, encoders: EncoderInfo | null): boolean {
    if (!encoders || !(codec in encoders.usable)) {
        return true;
    }
    return encoders.usable[codec];
}

/** The capture API to fall back to when the current one is unavailable here. */
function preferredCapture(platform: PlatformInfo | null): string {
    if (platform?.os === "linux") {
        return platform.display === "wayland" ? "portal" : "x11grab";
    }
    return "ddagrab";
}

// The GStreamer pipeline behind the portal backend targets a bitrate and
// negotiates its own pixel format, so it honors only the latency mode and
// ignores the chroma and color-range controls.
const PORTAL_MODE = "latency";
const PORTAL_UNIMPLEMENTED =
    "not yet implemented for the portal (GStreamer) capture backend";

/**
 * Controls and mode options the given capture backend does not honor, so the UI
 * greys them instead of accepting a value the engine would silently drop. The
 * portal backend is bitrate-only and format-agnostic.
 */
function backendUnsupported(capture: string): {
    modeOptions: Record<string, string>;
    controls: Record<string, string>;
} {
    if (capture === "portal") {
        return {
            modeOptions: { lossless: PORTAL_UNIMPLEMENTED, quality: PORTAL_UNIMPLEMENTED },
            controls: { chroma: PORTAL_UNIMPLEMENTED, colorRange: PORTAL_UNIMPLEMENTED },
        };
    }
    return { modeOptions: {}, controls: {} };
}

/** Reason chroma cannot be encoded by the codec with the given label. */
function chromaBlockReason(chroma: string, codecLabel: string): string {
    if (chroma === "gbrp") {
        return "direct RGB coding needs HEVC Range Extensions (hevc_nvenc)";
    }
    return `${codecLabel} cannot encode ${chroma}`;
}

/**
 * Evaluates which controls and individual options are unavailable for the given
 * settings. Every rule derives from the capability table (codec/chroma/transport
 * facts) and the domain meta tables (which control each mode uses), so the disable
 * rules cannot drift from the normalize repairs below. Pure: no React, no side
 * effects. Null platform/encoders/caps mean "unknown" and impose no restriction.
 */
export function evaluateDeps(
    s: Stream,
    platform: PlatformInfo | null,
    encoders: EncoderInfo | null = null,
    caps: Capability[] | null = null
): Deps {
    const mode = MODE_META[s.mode as Mode];
    const chroma = CHROMA_META[s.chroma as Chroma];
    const nvenc = isNvenc(s.codec, caps);

    const d: Deps = {
        disabled: {},
        optionDisabled: {
            codec: {},
            chroma: {},
            mode: {},
            capture: unavailableCaptures(platform),
        },
    };

    // A codec the current transport cannot carry, disabled per option.
    if (caps) {
        for (const cap of caps) {
            if (!cap.transports.includes(s.transport)) {
                const label = CODEC_META[cap.name as Codec]?.label ?? cap.name;
                d.optionDisabled.codec[cap.name] =
                    `${s.transport} cannot carry ${label}`;
            }
        }
        // A chroma the current codec cannot encode, disabled per option.
        const cap = findCapability(caps, s.codec);
        if (cap) {
            const label = CODEC_META[s.codec as Codec]?.label ?? s.codec;
            for (const c of Object.keys(CHROMA_META)) {
                if (!cap.chromas.includes(c)) {
                    d.optionDisabled.chroma[c] = chromaBlockReason(c, label);
                }
            }
        }
    }
    // Hardware availability wins over the transport reason: "no NVIDIA encoder"
    // is the actionable message when the codec cannot run here at all.
    Object.assign(d.optionDisabled.codec, unavailableCodecs(encoders));

    if (mode && !mode.usesCq) {
        d.disabled.cq = "the quantizer target only exists in quality mode";
    }
    if (mode && !mode.usesBitrate) {
        d.disabled.bitrateM =
            "lossless output costs whatever exactness costs - no bitrate bound";
    }
    if (!mode?.usesBframes || !nvenc) {
        d.disabled.bframes =
            "B-frames are forced off in lossless/latency modes (no gain, only reorder delay)";
    }
    if (!nvenc || mode?.pinsPreset) {
        d.disabled.encPreset = nvenc
            ? `latency mode pins the preset to ${mode?.pinnedPreset ?? "p5"}`
            : "the p1-p7 ladder is NVENC-specific";
    }
    if (chroma?.fullRange) {
        d.disabled.colorRange =
            "RGB is inherently full range - no quantization range choice exists";
    }
    // ddagrab selects an output by index; x11grab crops the X screen to the
    // monitor's geometry. The other backends do not take a monitor index.
    const monitorNa: Record<string, string> = {
        kmsgrab: "kmsgrab captures the whole scanout, not a single monitor",
        gdigrab: "gdigrab captures the whole desktop as one frame",
        portal: "the compositor's picker chooses the source, not a monitor index",
    };
    if (monitorNa[s.capture]) {
        d.disabled.monitor = monitorNa[s.capture];
    }

    // Controls the selected capture backend does not implement. Applied last so
    // the backend's "unimplemented" reason wins over a codec- or chroma-derived
    // one for the same control.
    const unsupported = backendUnsupported(s.capture);
    d.optionDisabled.mode = { ...d.optionDisabled.mode, ...unsupported.modeOptions };
    for (const [control, reason] of Object.entries(unsupported.controls)) {
        d.disabled[control] = reason;
    }

    return d;
}

/**
 * Applies availability fallbacks so settings never hold a combination the relay,
 * encoder or platform would reject. Returns a new object; never mutates the
 * input. Repairs derive from the same capability table as evaluateDeps, so a
 * disabled option and its fallback always agree. Null platform/encoders/caps
 * leave the corresponding dimension untouched.
 */
export function normalize(
    s: Stream,
    platform: PlatformInfo | null = null,
    encoders: EncoderInfo | null = null,
    caps: Capability[] | null = null
): Stream {
    const next = { ...s };

    // Codec: must run here (hardware) and be carriable by the transport. Walk the
    // table in display order and take the first codec that satisfies both.
    const codecOk = (codec: string): boolean =>
        codecUsable(codec, encoders) &&
        (!caps || (findCapability(caps, codec)?.transports.includes(next.transport) ?? false));
    if (!codecOk(next.codec)) {
        next.codec =
            (Object.keys(CODEC_META) as Codec[]).find(codecOk) ?? FALLBACK_CODEC;
    }

    // Chroma: must be encodable by the chosen codec.
    if (caps) {
        const cap = findCapability(caps, next.codec);
        if (cap && !cap.chromas.includes(next.chroma)) {
            next.chroma =
                CHROMA_FALLBACK_ORDER.find(c => cap.chromas.includes(c)) ??
                cap.chromas[0] ??
                next.chroma;
        }
    }

    if (unavailableCaptures(platform)[next.capture]) {
        next.capture = preferredCapture(platform);
    }

    // The portal backend runs the bitrate-only GStreamer pipeline, so a mode it
    // does not implement drops to latency. Applied after the capture repair,
    // which may itself select portal.
    if (next.capture === "portal" && next.mode !== PORTAL_MODE) {
        next.mode = PORTAL_MODE;
    }
    return next;
}
