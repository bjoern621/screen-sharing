import {
    Deps, EncoderInfo, PlatformInfo, Stream, TransportCarriage,
} from "../types/stream";
import {
    Capability, CHROMA_META, Chroma, Decoder, ENGINE_LABEL, Engine,
    FAMILY_META, Family, Knob, MODE_META, Mode, bitrateLimit, carriesFormat,
    chromaGapFor, codecLabel, cqMax, decodeNote, engineFor, engineGapFor,
    familiesWith, familyMetaOf, familyOf, findCapability, findEngineRule,
    modeGapFor,
} from "./domain";

/**
 * Everything outside the settings themselves that decides which combinations are
 * legal: the running platform, the machine's encoder probe, the backend
 * capability table, and per capture backend the transports its engine carries
 * and the engine that runs it. A null field means "not resolved yet" and imposes
 * no restriction, so the form behaves during startup as it does offline.
 *
 * The capture backend's engine reaches almost every rule here. It decides which
 * codecs the probe found, which pixel formats and rate-control modes the capability
 * table leaves reachable, and which transports can carry the stream, so the same
 * settings can be legal on one capture backend and repaired on another.
 *
 * Every transport in this module is the publish leg: these rules govern the
 * settings form, which configures publishing only. The watch leg is picked per
 * viewer and constrains nothing here.
 */
export interface Environment {
    platform: PlatformInfo | null;
    encoders: EncoderInfo | null;
    caps: Capability[] | null;
    /** The decode table, which describes the viewers rather than this machine and so
     * restricts nothing: it only lets the form say what a choice costs them. */
    decoders: Decoder[] | null;
    carriage: TransportCarriage[] | null;
    captureTransports: Record<string, string[]> | null;
    captureEngines: Record<string, string> | null;
}

/** An Environment with nothing resolved, for a call site that has no facts yet. */
export const UNKNOWN_ENV: Environment = {
    platform: null,
    encoders: null,
    caps: null,
    decoders: null,
    carriage: null,
    captureTransports: null,
    captureEngines: null,
};

// Repair chroma preference, highest quality first. normalize walks this order and
// picks the first format the repaired codec and the capture's engine both accept, so
// H.264's rejected gbrp drops to yuv444p and AV1's rejected 4:4:4 drops to yuv420p.
// It lists every chroma, so the walk finds one wherever the codec encodes anything
// at all on this engine.
const CHROMA_FALLBACK_ORDER: Chroma[] = [
    "yuv444p", "yuv420p", "p010le", "gbrp",
];

// Repair rate-control preference, quality first. normalize walks this order when the
// repaired codec has no form of the selected mode, so a stream on lossless x264 that
// switches to an AV1 encoder lands on constant quality rather than a bitrate target.
// It lists every mode, so the walk finds one wherever the codec is driveable on this
// engine at all.
const MODE_FALLBACK_ORDER: Mode[] = [
    "crf", "vbr", "abr", "cbr", "lossless",
];

/**
 * What each capture backend needs from the machine it runs on: the operating system,
 * and on Linux the session type, each with the sentence shown where the machine does
 * not have it. A backend with no `display` runs on either Linux session.
 *
 * One entry per backend, so backends reading the same screen source state the
 * requirement separately and cannot drift out of step: x11grab and ximagesrc read the
 * X screen through the same extension and differ only in which publish engine runs
 * them.
 */
const CAPTURE_NEEDS: Record<string, {
    os: string;
    display?: string;
    wrongOs: string;
    wrongSession?: string;
}> = {
    ddagrab: { os: "windows", wrongOs: "DXGI Desktop Duplication is Windows-only" },
    gdigrab: { os: "windows", wrongOs: "GDI capture is Windows-only" },
    kmsgrab: { os: "linux", wrongOs: "DRM/KMS capture is Linux-only" },
    x11grab: {
        os: "linux", display: "x11",
        wrongOs: "X11 capture is Linux-only",
        wrongSession: "Wayland session: x11grab only sees XWayland windows, not the Wayland desktop - use portal",
    },
    ximagesrc: {
        os: "linux", display: "x11",
        wrongOs: "X11 capture is Linux-only",
        wrongSession: "Wayland session: ximagesrc only sees XWayland windows, not the Wayland desktop - use portal",
    },
    portal: {
        os: "linux", display: "wayland",
        wrongOs: "PipeWire ScreenCast is Linux-only",
        wrongSession: "PipeWire ScreenCast needs a Wayland session",
    },
};

/**
 * Capture backends that cannot run on the given platform, each mapped to the reason.
 * A null platform (not yet detected) and an operating system no backend names impose
 * no restriction.
 */
function unavailableCaptures(
    platform: PlatformInfo | null
): Record<string, string> {
    if (!platform || (platform.os !== "linux" && platform.os !== "windows")) {
        return {};
    }
    const out: Record<string, string> = {};
    for (const [capture, need] of Object.entries(CAPTURE_NEEDS)) {
        if (platform.os !== need.os) {
            out[capture] = need.wrongOs;
        } else if (need.wrongSession && need.display && platform.display !== need.display) {
            out[capture] = need.wrongSession;
        }
    }
    return out;
}

/**
 * Codecs the probe could not run on the capture backend's engine, each mapped to the
 * reason. A null encoder set (probe not yet resolved), an unresolved engine or a codec
 * that engine never probes (libx264 on ffmpeg) imposes no restriction.
 *
 * A failed probe means different things per engine and family, and naming the wrong
 * one sends the user looking for the wrong fix. On the ffmpeg engine a hardware codec
 * needs a card the machine may not have and a software one needs its library compiled
 * into the build. On the GStreamer engine the element is missing from the registry
 * instead, either because its plugin is not installed or, for the hardware families,
 * because the plugin found no device to register it for.
 *
 * An engine the probe could not ask at all is a fourth answer, and it applies to every
 * codec rather than to one: a missing ffmpeg leaves no encoder reachable, the two the
 * probe assumes present included.
 */
function unavailableCodecs(
    encoders: EncoderInfo | null,
    engine: Engine | null,
    caps: Capability[] | null
): Record<string, string> {
    if (!encoders || !engine) {
        return {};
    }
    // Every codec the table names, under one reason. An unresolved capability table
    // leaves that list unknown, and an unknown list restricts nothing, as everywhere
    // else here.
    const engineWide = (reason: string): Record<string, string> =>
        Object.fromEntries((caps ?? []).map(c => [c.name, reason]));

    const unprobed = encoders.unprobed[engine];
    if (unprobed) {
        return engineWide(unprobed);
    }
    const probed = encoders.usable[engine];
    if (!probed) {
        // Detect answers every engine either with verdicts or with a reason, so this
        // is a contradiction rather than a missing fact. Reporting it beats treating
        // the engine as unrestricted, which would offer codecs nothing vouched for.
        return engineWide(
            `the ${ENGINE_LABEL[engine]} publish engine reported neither an encoder verdict nor a reason`
        );
    }
    const out: Record<string, string> = {};
    for (const [codec, ok] of Object.entries(probed)) {
        if (ok) {
            continue;
        }
        const family = familyMetaOf(codec, caps);
        if (!family) {
            // Which half is missing is the family's fact, and the capability table has
            // not arrived yet. The probe's verdict still holds, so the codec is greyed
            // under what is known rather than under a guessed half.
            out[codec] = `${ENGINE_LABEL[engine]} cannot run ${codec} on this machine`;
            continue;
        }
        // Whether an absent encoder is the machine's answer or the build's follows the
        // family, not the engine: a device family's encoder is missing because the
        // hardware or its driver is, a software one's because nobody compiled or
        // packaged it.
        if (engine === "gstreamer") {
            out[codec] = family.needsDevice
                ? `no ${family.label} encoder element registered - the GStreamer plugin found no such device`
                : `the GStreamer publish engine needs an encoder element for ${codec}, and this install carries no plugin providing one`;
            continue;
        }
        out[codec] = family.needsDevice
            ? `no ${family.label} encoder detected on this machine`
            : `this ffmpeg build has no ${codec} encoder compiled in`;
    }
    return out;
}

/**
 * Rate-control modes the selected codec cannot be driven in, each mapped to the
 * reason. The gaps come from the capability table, which keys them by publish
 * engine where only one builder reaches the mode: libvpx codes lossless VP9 and the
 * vp9enc element has no such property.
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

/**
 * Whether codec can run on the given engine here. An unresolved probe or engine, and a
 * codec that engine does not probe, count as usable. An engine the probe could not ask
 * runs nothing, so no codec counts as usable there and none is repaired onto it.
 */
function codecUsable(
    codec: string,
    engine: Engine | null,
    encoders: EncoderInfo | null
): boolean {
    if (!encoders || !engine) {
        return true;
    }
    if (encoders.unprobed[engine]) {
        return false;
    }
    const probed = encoders.usable[engine];
    return !probed || !(codec in probed) || probed[codec];
}

/** Shown for a codec or family the argument builders do not map yet. */
const notImplementedReason = "not implemented yet - on the roadmap";

/**
 * Encoder families with no selectable codec on this machine, each mapped to the
 * reason. A family is unavailable when none of its codecs is both implemented
 * and runnable here on the capture backend's engine; the reason distinguishes
 * "not built yet" from "no such hardware". Families with at least one usable codec
 * are absent (selectable).
 */
function unavailableFamilies(
    caps: Capability[],
    engine: Engine | null,
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
        const implemented = list.filter(c => c.implemented);
        if (implemented.length === 0) {
            out[family] = notImplementedReason;
            continue;
        }
        // A codec the current engine has no encoder for cannot make its family
        // selectable. Where that holds for the whole family, the family carries the
        // gap's own reason rather than the hardware one, which would send the user
        // shopping for a card that changes nothing.
        const gaps = implemented.map(c => engineGapFor(c.name, engine, caps));
        const firstGap = gaps[0];
        if (firstGap && gaps.every(Boolean)) {
            out[family] = firstGap.reason;
            continue;
        }
        const anyUsable = implemented.some(
            (c, i) => !gaps[i] && codecUsable(c.name, engine, encoders)
        );
        if (!anyUsable) {
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
 * Publish transports the given capture backend's publish engine cannot carry, each
 * mapped to the reason. The map (capture -> carriable transports) comes from the
 * backend; a transport known to some capture but absent from this one is disabled,
 * because that capture's engine has no publish sink for it. An unknown capture
 * imposes no restriction.
 */
function unavailableTransports(
    capture: string,
    engine: Engine | null,
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
            out[t] = engine
                ? `the ${capture} capture backend runs the ${ENGINE_LABEL[engine]} publish engine, which has no ${t} publish sink`
                : `the ${capture} capture backend cannot carry ${t}`;
        }
    }
    return out;
}

/** The capture backend to fall back to when the current one is unavailable here. */
function preferredCapture(platform: PlatformInfo | null): string {
    if (platform?.os === "linux") {
        return platform.display === "wayland" ? "portal" : "x11grab";
    }
    return "ddagrab";
}

/** Reason the codec with the given label cannot encode chroma on either engine.
 * Planar RGB gets its own wording: RGB reaches an encoder only through HEVC's
 * Range Extensions or VP9's identity matrix, which is a coding-tool fact rather
 * than a subsampling limit. */
function chromaBlockReason(chroma: string, label: string): string {
    if (chroma === "gbrp") {
        return `${label} codes no direct RGB - that needs HEVC Range Extensions or VP9's identity matrix`;
    }
    return `${label} cannot encode ${chroma}`;
}

/**
 * Pixel formats the current codec cannot be handed, each mapped to the reason.
 * Two facts block a format, and the capability table carries both: a format the
 * codec's encoder codes on no engine is absent from its `chromas`, and one only
 * the other engine's encoder takes carries a gap naming that engine. The gap's
 * reason is shown as it stands, since it already says which capture backends
 * reach the format. evaluateDeps greys these and normalize repairs away from
 * them, so the two cannot disagree.
 */
function unavailableChromas(
    codec: string,
    engine: Engine | null,
    caps: Capability[] | null
): Record<string, string> {
    const out: Record<string, string> = {};
    const cap = findCapability(caps, codec);
    if (!cap) {
        return out;
    }
    const label = codecLabel(cap);
    for (const c of Object.keys(CHROMA_META) as Chroma[]) {
        if (!cap.chromas.includes(c)) {
            out[c] = chromaBlockReason(c, label);
            continue;
        }
        const gap = chromaGapFor(codec, engine, c, caps);
        if (gap) {
            out[c] = gap.reason;
        }
    }
    return out;
}

/** The publish engine that runs the capture backend, or null while unknown. */
function engineOf(capture: string, env: Environment): Engine | null {
    return engineFor(capture, env.captureEngines);
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
    const { platform, encoders, caps, decoders, carriage, captureTransports } = env;
    const mode = MODE_META[s.mode as Mode];
    const chroma = CHROMA_META[s.chroma as Chroma];
    // The settings fields the codec's encoder family owns. An unresolved capability
    // table imposes no restriction here as everywhere else, so the two controls stay
    // live during startup instead of greying and flipping back once it arrives.
    const family = familyMetaOf(s.codec, caps);
    const takesBframes = !caps || !!family?.takesBframes;
    const takesPreset = !caps || !!family?.takesPreset;
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
    // option: a transport the engine cannot serialize is greyed with the reason
    // rather than left to fail at launch.
    if (captureTransports) {
        d.optionDisabled.transport = unavailableTransports(s.capture, engine, captureTransports);
    }

    // A codec whose bitstream the current publish transport has no mapping for,
    // disabled per option. The protocol carries a format rather than an encoder,
    // so every codec of that format greys out together.
    if (caps) {
        for (const cap of caps) {
            if (!carriesFormat(carriage, s.transport, "publish", cap.format)) {
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
        // A codec the capture backend's engine has no encoder for at all. It stays
        // in the list, since the other engine's capture backends publish it, and
        // the gap's reason says which side lacks it.
        for (const cap of caps) {
            const gap = engineGapFor(cap.name, engine, caps);
            if (gap) {
                d.optionDisabled.codec[cap.name] = gap.reason;
            }
        }
        // Encoder families (the first dropdown) with no selectable codec here.
        d.optionDisabled.family = unavailableFamilies(caps, engine, encoders);
    }
    // A failed probe wins over the transport reason: "no NVIDIA encoder" and "not
    // compiled into this ffmpeg" are the actionable messages when the codec cannot
    // run here at all. Probed codecs are all implemented, so this never overwrites
    // the roadmap reason.
    Object.assign(d.optionDisabled.codec, unavailableCodecs(encoders, engine, caps));

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
    // Which families own the two fields is FAMILY_META's, so the reason lists them
    // from the table rather than naming one.
    knob("bframes", !!mode?.usesBframes && takesBframes,
        mode?.usesBframes
            ? `only the ${familiesWith(m => !!m.takesBframes)} encoders take a B-frame count`
            : "B-frames are forced off in CBR and lossless (no gain, only reorder delay)");
    // A mode that pins the preset carries the step it pins to, so the sentence reads
    // the declared value instead of restating one.
    knob("encPreset", takesPreset && !mode?.pinsPreset,
        takesPreset && mode?.pinsPreset
            ? `${s.mode.toUpperCase()} pins the preset to ${mode.pinnedPreset}`
            : `only the ${familiesWith(m => !!m.takesPreset)} encoders take an encoder preset`);
    // The keyframe interval is not a rate-control concept, so no mode withholds it;
    // only an encoder element that has no property for it does.
    knob("gop", true, "");

    // The va elements express a VBR target as a percentage of the ceiling and take
    // 50% at the lowest, so a ceiling more than twice the target has no form there
    // and the GStreamer builder refuses it. The knob is forwarded, so it stays
    // live and carries the bound instead of being greyed.
    if (
        engine === "gstreamer" &&
        s.mode === "vbr" &&
        familyOf(s.codec, caps) === "vaapi" &&
        s.bitrateM > 0 &&
        s.maxrateM > s.bitrateM * 2
    ) {
        d.note.maxrateM = `the VAAPI encoder elements place the target as a percentage of the ceiling, at 50% lowest, so this ceiling cannot exceed ${s.bitrateM * 2} Mbit/s against a ${s.bitrateM} Mbit/s target`;
    }

    // What the pixel format costs the viewer. The chroma control is never greyed for
    // this: every format has a software decoder, so the choice is between a viewer's
    // GPU and a viewer's cores, and which one is a fact the publisher should see rather
    // than a rule that overrides the choice.
    d.note.chroma = decodeNote(decoders, s.codec, s.chroma as Chroma, caps);

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
 * Moves settings off the combinations the relay, encoder or platform would reject,
 * onto the first one those same tables accept. Returns a new object; never mutates
 * the input. Repairs derive from the same tables as evaluateDeps, so a disabled
 * option and its replacement always agree. An unresolved Environment field leaves
 * the corresponding dimension untouched.
 *
 * A dimension with nothing available is left holding the rejected value rather than
 * moved to a guess. Every candidate here has to satisfy the same rules evaluateDeps
 * greys an option by, so a value picked outside them would be a value the form
 * greys and the publish refuses, which is the one state the two are supposed to
 * never disagree about. The rejected value keeps its reason, and the publish refuses
 * with it.
 */
export function normalize(s: Stream, env: Environment = UNKNOWN_ENV): Stream {
    const { platform, encoders, caps, carriage, captureTransports } = env;
    const next = { ...s };

    // Capture first: the transport, codec and chroma repairs below depend on it, so
    // it settles to a backend this platform can run before they read it, and the
    // engine that backend runs on is fixed from here.
    if (unavailableCaptures(platform)[next.capture]) {
        next.capture = preferredCapture(platform);
    }
    const engine = engineOf(next.capture, env);

    // Transport: the capture backend's engine must be able to carry it, so a
    // capture change can strand the transport; fall back to the first transport
    // that capture can carry.
    if (captureTransports) {
        const allowed = captureTransports[next.capture];
        if (allowed && allowed.length > 0 && !allowed.includes(next.transport)) {
            next.transport = allowed[0];
        }
    }

    // Codec: must be implemented, have an encoder on the capture backend's engine,
    // run here (hardware) and produce a bitstream the transport carries. Walk the
    // capability table in display order and take the first codec that satisfies all
    // four.
    const codecOk = (codec: string): boolean => {
        if (!codecUsable(codec, engine, encoders)) {
            return false;
        }
        if (!caps) {
            return true;
        }
        const cap = findCapability(caps, codec);
        return (
            !!cap && cap.implemented &&
            carriesFormat(carriage, next.transport, "publish", cap.format) &&
            !engineGapFor(codec, engine, caps)
        );
    };
    // No codec satisfying all four means this capture backend's engine can encode
    // nothing here, which is what evaluateDeps says on the codec field. Naming one
    // anyway would name a codec that same evaluation greys.
    if (!codecOk(next.codec)) {
        const usable = caps?.find(c => codecOk(c.name));
        if (usable) {
            next.codec = usable.name;
        }
    }

    // Chroma: must be a format the chosen codec encodes and the capture backend's
    // engine accepts, so switching to the portal backend moves gbrp to the 4:4:4 its
    // encoders negotiate rather than to a silent conversion.
    const blocked = unavailableChromas(next.codec, engine, caps);
    if (blocked[next.chroma]) {
        const free = CHROMA_FALLBACK_ORDER.find(c => !blocked[c]);
        if (free) {
            next.chroma = free;
        }
    }

    // Rate control: the chosen codec's encoder must have the mode on the engine
    // that will build the command, so moving a lossless stream onto an AV1 encoder,
    // or a lossless VP9 one onto the GStreamer engine, settles on a mode that exists
    // instead of failing at launch.
    const blockedModes = unavailableModes(next.codec, engine, caps);
    if (blockedModes[next.mode]) {
        const free = MODE_FALLBACK_ORDER.find(m => !blockedModes[m]);
        if (free) {
            next.mode = free;
        }
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
