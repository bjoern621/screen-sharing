import { Deps, EncoderInfo, PlatformInfo, Stream } from "../types/stream";
import {
    Capability, CHROMA_META, Chroma, FALLBACK_CODEC, FAMILY_META, Family,
    MODE_META, Mode, codecLabel, findCapability, isNvenc,
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

/** Shown for a codec or family the argument builders do not map yet. */
const notImplementedReason = "not implemented yet - on the roadmap";

/**
 * Encoder families with no selectable codec on this machine, each mapped to the
 * reason. A family is unavailable when none of its codecs is both implemented
 * and runnable here; the reason distinguishes "not built yet" from "no such
 * hardware". Families with at least one usable codec are absent (selectable).
 */
function unavailableFamilies(
    caps: Capability[],
    encoders: EncoderInfo | null
): Record<string, string> {
    const byFamily = new Map<string, Capability[]>();
    for (const c of caps) {
        const list = byFamily.get(c.family) ?? [];
        list.push(c);
        byFamily.set(c.family, list);
    }
    const out: Record<string, string> = {};
    for (const [family, list] of byFamily) {
        const anyImplemented = list.some(c => c.implemented);
        const anyUsable = list.some(
            c => c.implemented && codecUsable(c.name, encoders)
        );
        if (!anyImplemented) {
            out[family] = notImplementedReason;
        } else if (!anyUsable) {
            const label = FAMILY_META[family as Family]?.label ?? family;
            out[family] = `no ${label} encoder detected on this machine`;
        }
    }
    return out;
}

/**
 * Audio sources the given platform cannot capture, each mapped to the reason.
 * Desktop audio comes from the PulseAudio/PipeWire monitor source; ffmpeg has
 * no WASAPI loopback, so Windows has no equivalent.
 */
function unavailableAudio(
    platform: PlatformInfo | null
): Record<string, string> {
    if (platform?.os === "windows") {
        return {
            desktop:
                "desktop audio capture needs PulseAudio/PipeWire (Linux) - ffmpeg has no WASAPI loopback on Windows",
        };
    }
    return {};
}

/**
 * Transports the given capture backend's engine cannot carry, each mapped to the
 * reason. The map (capture -> carriable transports) comes from the backend; a
 * transport known to some capture but absent from this one is disabled, because
 * that capture's engine has no sink for it (the portal/GStreamer path and
 * WebRTC). An unknown capture imposes no restriction.
 */
function unavailableTransports(
    capture: string,
    captureTransports: Record<string, string[]>
): Record<string, string> {
    const allowed = captureTransports[capture];
    if (!allowed) {
        return {};
    }
    const all = new Set<string>();
    for (const list of Object.values(captureTransports)) {
        for (const t of list) {
            all.add(t);
        }
    }
    const out: Record<string, string> = {};
    for (const t of all) {
        if (!allowed.includes(t)) {
            out[t] = `the ${capture} capture path cannot carry ${t}`;
        }
    }
    return out;
}

/** The capture API to fall back to when the current one is unavailable here. */
function preferredCapture(platform: PlatformInfo | null): string {
    if (platform?.os === "linux") {
        return platform.display === "wayland" ? "portal" : "x11grab";
    }
    return "ddagrab";
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
    caps: Capability[] | null = null,
    captureTransports: Record<string, string[]> | null = null
): Deps {
    const mode = MODE_META[s.mode as Mode];
    const chroma = CHROMA_META[s.chroma as Chroma];
    const nvenc = isNvenc(s.codec, caps);

    const d: Deps = {
        disabled: {},
        optionDisabled: {
            family: {},
            codec: {},
            chroma: {},
            mode: {},
            transport: {},
            capture: unavailableCaptures(platform),
            audio: unavailableAudio(platform),
        },
    };

    // Transports the selected capture's engine cannot carry, disabled per
    // option. The portal path runs through GStreamer, which has no WebRTC sink,
    // so a transport the engine cannot serialize is greyed with the reason
    // rather than left to fail at launch.
    if (captureTransports) {
        d.optionDisabled.transport = unavailableTransports(s.capture, captureTransports);
    }

    // A codec the current transport cannot carry, disabled per option.
    if (caps) {
        for (const cap of caps) {
            if (!cap.transports.includes(s.transport)) {
                d.optionDisabled.codec[cap.name] =
                    `${s.transport} cannot carry ${codecLabel(cap)}`;
            }
        }
        // A chroma the current codec cannot encode, disabled per option.
        const cap = findCapability(caps, s.codec);
        if (cap) {
            const label = codecLabel(cap);
            for (const c of Object.keys(CHROMA_META)) {
                if (!cap.chromas.includes(c)) {
                    d.optionDisabled.chroma[c] = chromaBlockReason(c, label);
                }
            }
        }
        // Codecs the encoder argument builders do not map yet: shown so the
        // roadmap is visible, but greyed with the reason.
        for (const cap of caps) {
            if (!cap.implemented) {
                d.optionDisabled.codec[cap.name] = notImplementedReason;
            }
        }
        // Encoder families (the first dropdown) with no selectable codec here.
        d.optionDisabled.family = unavailableFamilies(caps, encoders);
    }
    // Hardware availability wins over the transport reason: "no NVIDIA encoder"
    // is the actionable message when the codec cannot run here at all. Probed
    // codecs are all implemented, so this never overwrites the roadmap reason.
    Object.assign(d.optionDisabled.codec, unavailableCodecs(encoders));

    if (mode && !mode.usesCq) {
        d.disabled.cq = "the quantizer target only exists in CRF (constant-quality) mode";
    }
    if (mode && !mode.usesBitrate) {
        d.disabled.bitrateM =
            "constant-quality and lossless set no bitrate target";
    }
    if (mode && !mode.usesMaxrate) {
        d.disabled.maxrateM =
            "the burst ceiling exists only in constrained VBR";
    }
    if (mode && !mode.usesVbv) {
        d.disabled.vbvMs =
            "the VBV buffer bounds the rate only in CBR and VBR";
    }
    if (!mode?.usesBframes || !nvenc) {
        d.disabled.bframes =
            "B-frames are forced off in CBR and lossless (no gain, only reorder delay)";
    }
    if (!nvenc || mode?.pinsPreset) {
        d.disabled.encPreset = nvenc
            ? `CBR mode pins the preset to ${mode?.pinnedPreset ?? "p5"}`
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
    caps: Capability[] | null = null,
    captureTransports: Record<string, string[]> | null = null
): Stream {
    const next = { ...s };

    // Capture first: the transport and codec repairs below depend on it, so it
    // settles to a backend this platform can run before they read it.
    if (unavailableCaptures(platform)[next.capture]) {
        next.capture = preferredCapture(platform);
    }

    // Transport: the capture backend's engine must be able to carry it. The
    // portal (GStreamer) path has no WebRTC sink, so a capture change can strand
    // the transport; fall back to the first transport that capture can carry.
    if (captureTransports) {
        const allowed = captureTransports[next.capture];
        if (allowed && allowed.length > 0 && !allowed.includes(next.transport)) {
            next.transport = allowed[0];
        }
    }

    // Codec: must be implemented, run here (hardware) and be carriable by the
    // transport. Walk the capability table in display order and take the first
    // codec that satisfies all three; fall back to software when none does.
    const codecOk = (codec: string): boolean => {
        if (!codecUsable(codec, encoders)) {
            return false;
        }
        if (!caps) {
            return true;
        }
        const cap = findCapability(caps, codec);
        return (
            !!cap && cap.implemented && cap.transports.includes(next.transport)
        );
    };
    if (!codecOk(next.codec)) {
        next.codec =
            (caps?.map(c => c.name).find(codecOk)) ?? FALLBACK_CODEC;
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

    // Audio: settings and presets from before the option lack the key, and
    // desktop capture has no Windows path.
    if (!next.audio || unavailableAudio(platform)[next.audio]) {
        next.audio = "none";
    }

    return next;
}
