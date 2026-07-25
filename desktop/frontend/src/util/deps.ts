import { Deps, EncoderInfo, PlatformInfo, Stream } from "../types/stream";
import {
    Capability, CHROMA_META, Chroma, Engine, FALLBACK_CODEC, FAMILY_META, Family,
    Knob, MODE_META, Mode, bitrateLimit, codecLabel, cqMax, findCapability,
    findEngineRule, isNvenc, modeGapFor,
} from "./domain";

/**
 * Everything outside the settings themselves that decides which combinations are
 * legal: the running platform, the machine's encoder probe, the backend
 * capability table, and per capture backend the transports its engine carries
 * and the engine that runs it. A null field means "not resolved yet" and imposes
 * no restriction, so the form behaves during startup as it does offline.
 *
 * Every transport in this module is the publish leg: these rules govern the
 * settings form, which configures publishing only. The watch leg is picked per
 * viewer and constrains nothing here.
 */
export interface Environment {
    platform: PlatformInfo | null;
    encoders: EncoderInfo | null;
    caps: Capability[] | null;
    captureTransports: Record<string, string[]> | null;
    captureEngines: Record<string, string> | null;
}

/** An Environment with nothing resolved, for a call site that has no facts yet. */
export const UNKNOWN_ENV: Environment = {
    platform: null,
    encoders: null,
    caps: null,
    captureTransports: null,
    captureEngines: null,
};

// Fallback chroma preference, highest quality first. normalize walks this order
// and picks the first format the repaired codec and the capture's engine both
// accept, so H.264's rejected gbrp drops to yuv444p and AV1's rejected 4:4:4
// drops to yuv420p. It lists every chroma, so the walk always finds one.
const CHROMA_FALLBACK_ORDER: Chroma[] = [
    "yuv444p", "yuv420p", "p010le", "gbrp",
];

// Fallback rate-control preference, quality first. normalize walks this order when
// the repaired codec has no form of the selected mode, so a stream on lossless
// x264 that switches to an AV1 encoder lands on constant quality rather than a
// bitrate target. It lists every mode, so the walk always finds one.
const MODE_FALLBACK_ORDER: Mode[] = [
    "crf", "vbr", "abr", "cbr", "lossless",
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
 *
 * A failed probe means two different things, so the reason follows the family: a
 * hardware codec needs a card the machine may not have, while a software one needs
 * its library compiled into the ffmpeg build. Naming the wrong one sends the user
 * looking for the wrong fix.
 */
function unavailableCodecs(
    encoders: EncoderInfo | null,
    caps: Capability[] | null
): Record<string, string> {
    if (!encoders) {
        return {};
    }
    const out: Record<string, string> = {};
    for (const [codec, ok] of Object.entries(encoders.usable)) {
        if (ok) {
            continue;
        }
        const family = findCapability(caps, codec)?.family;
        const label = FAMILY_META[family as Family]?.label;
        out[codec] =
            family === "software"
                ? `this ffmpeg build has no ${codec} encoder compiled in`
                : `no ${label ?? "matching"} encoder detected on this machine`;
    }
    return out;
}

/**
 * Rate-control modes the selected codec cannot be driven in, each mapped to the
 * reason. The gaps come from the capability table, which keys them by publish
 * engine where only one builder reaches the mode: libvpx codes lossless VP9 and
 * the portal path's vp9enc has no such property.
 */
function unavailableModes(
    codec: string,
    engine: Engine | null,
    caps: Capability[] | null
): Record<string, string> {
    const out: Record<string, string> = {};
    for (const mode of Object.keys(MODE_META) as Mode[]) {
        const gap = modeGapFor(codec, engine, mode, caps);
        if (gap) {
            out[mode] = gap.reason;
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
 * Publish transports the given capture backend's engine cannot carry, each mapped
 * to the reason. The map (capture -> carriable transports) comes from the backend; a
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

/** Reason the codec with the given label cannot encode chroma. Planar RGB gets
 * its own wording: RGB reaches an encoder only through HEVC's Range Extensions
 * or VP9's identity matrix, which is a coding-tool fact rather than a
 * subsampling limit. */
function chromaBlockReason(chroma: string, label: string): string {
    if (chroma === "gbrp") {
        return `${label} codes no direct RGB - that needs HEVC Range Extensions or VP9's identity matrix`;
    }
    return `${label} cannot encode ${chroma}`;
}

/**
 * Pixel formats the current codec cannot be handed, each mapped to the reason:
 * the formats its capability entry omits, plus planar RGB on the GStreamer
 * engine, whose encoders negotiate none. evaluateDeps greys these and normalize
 * repairs away from them, so the two cannot disagree.
 */
function unavailableChromas(
    codec: string,
    engine: Engine | null,
    caps: Capability[] | null
): Record<string, string> {
    const out: Record<string, string> = {};
    const cap = findCapability(caps, codec);
    if (cap) {
        const label = codecLabel(cap);
        for (const c of Object.keys(CHROMA_META)) {
            if (!cap.chromas.includes(c)) {
                out[c] = chromaBlockReason(c, label);
            }
        }
    }
    // x264enc, x265enc, vp9enc and the nvcodec elements all negotiate YUV only,
    // so the portal pipeline converts planar RGB to 4:4:4 before the encoder
    // (gstChromaFormat). Picking it there would cost RGB's bitrate without its
    // exactness.
    if (engine === "gstreamer") {
        out.gbrp =
            "the portal path's GStreamer encoders take no planar RGB - the pipeline would convert it to 4:4:4 YUV";
    }
    return out;
}

/** The media engine that runs the capture backend, or null while unknown. */
function engineOf(capture: string, env: Environment): Engine | null {
    const name = env.captureEngines?.[capture];
    return name === "ffmpeg" || name === "gstreamer" ? name : null;
}

/**
 * Evaluates which controls and individual options are unavailable for the given
 * settings. Every rule derives from the capability table (codec/chroma/transport
 * facts), the domain meta tables (which control each mode uses) and the engine
 * rules (which knob each builder forwards), so the disable rules cannot drift
 * from the normalize repairs below. Pure: no React, no side effects. An
 * unresolved Environment field imposes no restriction.
 */
export function evaluateDeps(s: Stream, env: Environment = UNKNOWN_ENV): Deps {
    const { platform, encoders, caps, captureTransports } = env;
    const mode = MODE_META[s.mode as Mode];
    const chroma = CHROMA_META[s.chroma as Chroma];
    const nvenc = isNvenc(s.codec, caps);
    const engine = engineOf(s.capture, env);

    const d: Deps = {
        disabled: {},
        note: {},
        optionDisabled: {
            family: {},
            codec: {},
            chroma: unavailableChromas(s.codec, engine, caps),
            mode: unavailableModes(s.codec, engine, caps),
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
    // A failed probe wins over the transport reason: "no NVIDIA encoder" and "not
    // compiled into this ffmpeg" are the actionable messages when the codec cannot
    // run here at all. Probed codecs are all implemented, so this never overwrites
    // the roadmap reason.
    Object.assign(d.optionDisabled.codec, unavailableCodecs(encoders, caps));

    // A rate-control control is live when three facts agree: the mode's concept
    // uses the knob, the codec's encoder has it, and the capture backend's engine
    // forwards the value. An engine that forwards a knob the mode marks unused
    // leaves a note instead, so the field states what the value does there rather
    // than feeding the encoder a number the form never showed.
    const knob = (id: Knob, uses: boolean, reason: string) => {
        const rule = findEngineRule(id, engine, s.codec, s.mode as Mode, caps);
        if (rule?.forwards) {
            d.note[id] = rule.reason;
        } else if (rule && !rule.modes) {
            // An engine that drops the knob in every mode outranks the mode's own
            // reason: no rate control brings the control back here, so naming the
            // mode would send the user hunting for a switch that changes nothing.
            d.disabled[id] = rule.reason;
        } else if (!uses) {
            d.disabled[id] = reason;
        } else if (rule) {
            d.disabled[id] = rule.reason;
        }
    };

    knob("cq", !!mode?.usesCq,
        "the quantizer target only exists in CRF (constant-quality) mode");
    knob("bitrateM", !!mode?.usesBitrate,
        "constant-quality and lossless set no bitrate target");
    knob("maxrateM", !!mode?.usesMaxrate,
        "the burst ceiling exists only in constrained VBR");
    knob("vbvMs", !!mode?.usesVbv,
        "the VBV buffer bounds the rate only in CBR and VBR");
    // B-frames and the preset ladder are each blocked by two independent facts,
    // so the reason names the one that applies instead of always blaming the mode.
    knob("bframes", !!mode?.usesBframes && nvenc,
        mode?.usesBframes
            ? "only the NVENC encoders take a B-frame count"
            : "B-frames are forced off in CBR and lossless (no gain, only reorder delay)");
    knob("encPreset", nvenc && !mode?.pinsPreset,
        nvenc
            ? `CBR pins the preset to ${mode?.pinnedPreset ?? "p5"}`
            : "the p1-p7 ladder is NVENC-specific");
    // The keyframe interval is not a rate-control concept, so no mode withholds it;
    // only an encoder element that has no property for it does.
    knob("gop", true, "");

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
 * input. Repairs derive from the same tables as evaluateDeps, so a disabled
 * option and its fallback always agree. An unresolved Environment field leaves
 * the corresponding dimension untouched.
 */
export function normalize(s: Stream, env: Environment = UNKNOWN_ENV): Stream {
    const { platform, encoders, caps, captureTransports } = env;
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

    // Chroma: must be a format the chosen codec encodes and the capture
    // backend's engine accepts, so switching to the portal path moves gbrp to the
    // 4:4:4 its encoders negotiate rather than to a silent conversion.
    const engine = engineOf(next.capture, env);
    const blocked = unavailableChromas(next.codec, engine, caps);
    if (blocked[next.chroma]) {
        next.chroma = CHROMA_FALLBACK_ORDER.find(c => !blocked[c]) ?? next.chroma;
    }

    // Rate control: the chosen codec's encoder must have the mode on the engine
    // that will build the command, so moving a lossless stream onto an AV1 encoder,
    // or a lossless VP9 one onto the portal path, settles on a mode that exists
    // instead of failing at launch.
    const blockedModes = unavailableModes(next.codec, engine, caps);
    if (blockedModes[next.mode]) {
        next.mode =
            MODE_FALLBACK_ORDER.find(m => !blockedModes[m]) ?? next.mode;
    }

    // Quantizer target: the constant-quality scales differ per encoder, so a
    // value carried over from libvpx's 63-point scale is clamped into the 51 the
    // H.26x and AV1 encoders accept. It is left alone while the table is
    // unresolved, which would otherwise clamp a saved VP9 value against the
    // fallback scale at startup.
    if (caps) {
        next.cq = Math.min(Math.max(next.cq, 0), cqMax(next.codec, caps));
        // Bitrate target: one encoder rejects a target above its ceiling instead of
        // clamping, and the defaults sit above that ceiling, so the value follows the
        // codec down. The burst ceiling rides along, since a maxrate below the target
        // would leave constrained VBR no room.
        const limit = bitrateLimit(next.codec, caps);
        if (limit > 0 && next.bitrateM > limit) {
            next.bitrateM = limit;
            next.maxrateM = Math.max(next.maxrateM, limit);
        }
    }

    // Audio: settings and presets from before the option lack the key, and
    // desktop capture has no Windows path.
    if (!next.audio || unavailableAudio(platform)[next.audio]) {
        next.audio = "none";
    }

    return next;
}
