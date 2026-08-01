import { Monitor, Option } from "../types/stream";
import {
    AUDIO_CODEC_META, AUDIO_META, AudioCodec, CHROMA_META, Capability,
    ENCODER_TIPS, ENGINE_LABEL, Engine, FAMILY_META, FORMAT_META, Family, Format,
    MODE_META, bitrateLimit, cqMax, metaOptions, scaleCq,
} from "./domain";

const NVENC_LINK = "https://en.wikipedia.org/wiki/Nvidia_NVENC";

// Mode and chroma option lists are derived from the domain meta tables so the
// dropdowns, the dependency rules and the heuristics share one definition. The
// encoder (family) and codec (format) lists come from the backend capability
// table instead, so they list exactly the codecs the backend declares.
export const MODES: Option[] = metaOptions(MODE_META);
export const CHROMAS: Option[] = metaOptions(CHROMA_META);
export const AUDIO_SOURCES: Option[] = metaOptions(AUDIO_META);

/**
 * The "Audio codec" dropdown: one option per codec the backend audio table
 * declares, in table order. Empty until the table loads.
 *
 * The rate and bitrate the track is coded at are the backend's figures, so the
 * tooltip reads them off the row instead of restating them here: the form cannot
 * promise a bitrate the encoder is not given.
 */
export function audioCodecOptions(codecs: AudioCodec[] | null): Option[] {
    if (!codecs) {
        return [];
    }
    return codecs.map(a => {
        const m = AUDIO_CODEC_META[a.name];
        const coded = `Coded as a stereo track at ${a.rate / 1000} kHz, ${a.bitrateK} kbit/s.`;
        return {
            value: a.name,
            label: m?.label ?? a.name,
            tip: m ? `${m.tip}\n${coded}` : coded,
            link: m?.link,
        };
    });
}

/**
 * The "Encoder family" dropdown: one option per encoder family present in the
 * capability table, in table order. Empty until the table loads.
 */
export function familyOptions(caps: Capability[] | null): Option[] {
    if (!caps) {
        return [];
    }
    const seen = new Set<string>();
    const out: Option[] = [];
    for (const c of caps) {
        if (seen.has(c.family)) {
            continue;
        }
        seen.add(c.family);
        const m = FAMILY_META[c.family as Family];
        out.push({ value: c.family, label: m?.label ?? c.family, tip: m?.tip, link: m?.link });
    }
    return out;
}

/**
 * The "Video codec" dropdown for a chosen family: one option per codec in that
 * family, labeled by its video format. Where a family holds several encoders for
 * one format, as the software family does for AV1, the label carries the encoder
 * name too, since the format alone would name them all the same. The tooltip picks
 * up what that encoder adds over its format (ENCODER_TIPS). Empty until the table
 * loads.
 */
export function codecOptions(family: string, caps: Capability[] | null): Option[] {
    if (!caps) {
        return [];
    }
    const inFamily = caps.filter(c => c.family === family);
    return inFamily.map(c => {
        const m = FORMAT_META[c.format as Format];
        const format = m?.label ?? c.format;
        const shared = inFamily.filter(o => o.format === c.format).length > 1;
        const detail = ENCODER_TIPS[c.name];
        return {
            value: c.name,
            label: shared ? `${format} (${c.name})` : format,
            tip: detail ? `${m?.tip ?? format}\n${detail}` : m?.tip,
            link: m?.link,
        };
    });
}

/**
 * The publish engine readout beside the capture backend: its display name, or a
 * placeholder while the capture-to-engine map is still loading.
 */
export function engineValue(engine: Engine | null): string {
    return engine ? ENGINE_LABEL[engine] : "resolving...";
}

/**
 * Tooltip for the publish engine readout. The engine is not a setting: it follows
 * from the capture backend, so the text says what it governs and how to change it.
 * Without it a user meeting "reachable on the ffmpeg publish engine only" in a greyed
 * option has no way to learn which engine is running.
 */
export function engineTip(engine: Engine | null): string {
    const lines = [
        "Media framework that runs capture, encode and publish in one process. It follows from the capture backend, so switching backend switches engine.",
        "The engine decides which encoders are installed, which pixel formats and rate-control knobs those encoders expose, and which transports can carry the stream. An option the engine cannot reach is greyed with the reason.",
    ];
    if (engine === "gstreamer") {
        lines.push("GStreamer runs the pipeline shown as the command below, one element per stage.");
    } else if (engine === "ffmpeg") {
        lines.push("ffmpeg runs the single command shown below.");
    }
    return lines.join("\n");
}

export const RANGES: Option[] = [
    {
        value: "pc", label: "pc - full range (0–255)",
        link: "https://en.wikipedia.org/wiki/YCbCr",
        tip: "Full range: all code values carry image data. Correct for computer graphics; mismatch causes crushed or washed tones.\nA desktop is full-range RGB to begin with, so this is the range that reaches a viewer unchanged. Both publish engines write the range into the bitstream, and every viewer here reads it.",
    },
    {
        value: "tv", label: "tv - limited / studio swing (16–235)",
        link: "https://en.wikipedia.org/wiki/Broadcast-safe",
        tip: "Limited/studio swing, a broadcast legacy. Only pick when a downstream device demands it (or for maximum web-player compatibility).\nDesktop content is squeezed into 16-235 on the way in and expanded again on the way out, so it loses code values and viewers land on slightly different ones: the native grid renders a limited-range mid grey about two levels below what ffplay and mpv render.",
    },
];

// Each capture backend states the publish engine it runs on, because that engine
// decides which codecs, pixel formats and rate-control knobs the rest of the form
// offers. The engine is never picked directly, so this is the only place a user can
// read the connection between the two.
export const CAPTURES: Option[] = [
    {
        value: "ddagrab", label: "ddagrab - DXGI Desktop Duplication",
        link: "https://learn.microsoft.com/en-us/windows/win32/direct3ddxgi/desktop-dup-api",
        tip: "DXGI Desktop Duplication (Windows): captures the composited framebuffer on the GPU, per monitor. Preferred on Windows.\nRuns the ffmpeg publish engine.",
    },
    {
        value: "gdigrab", label: "gdigrab - GDI BitBlt",
        link: "https://learn.microsoft.com/en-us/windows/win32/api/wingdi/nf-wingdi-bitblt",
        tip: "GDI BitBlt (Windows): CPU copy of the whole desktop - all monitors as ONE frame; multi-monitor widths can exceed NVENC's 8192 px limit.\nRuns the ffmpeg publish engine.",
    },
    {
        value: "d3d11screencapturesrc", label: "d3d11screencapturesrc - Desktop Duplication (GStreamer)",
        link: "https://gstreamer.freedesktop.org/documentation/d3d11/d3d11screencapturesrc.html",
        tip: "Direct3D 11 screen capture (Windows): the same Desktop Duplication surface ddagrab reads, taken by GStreamer instead, selected by monitor index with the cursor drawn in.\nRuns the GStreamer publish engine, which is what puts that engine's encoders and its WebRTC sink within reach on Windows.",
    },
    {
        value: "x11grab", label: "x11grab - X11 SHM",
        link: "https://ffmpeg.org/ffmpeg-devices.html#x11grab",
        tip: "X11 shared-memory capture (Linux, also XWayland windows). Default on Linux; pure-Wayland surfaces need the portal capture backend instead.\nRuns the ffmpeg publish engine.",
    },
    {
        value: "ximagesrc", label: "ximagesrc - X11 SHM (GStreamer)",
        link: "https://gstreamer.freedesktop.org/documentation/ximagesrc/index.html",
        tip: "X11 shared-memory capture (Linux, also XWayland windows), the same screen source as x11grab read by GStreamer instead. Crops to the selected monitor and paces itself, so no damage-driven repeat is involved.\nRuns the GStreamer publish engine: pick it over x11grab to reach that engine's encoders on an X11 session.",
    },
    {
        value: "kmsgrab", label: "kmsgrab - DRM/KMS",
        link: "https://en.wikipedia.org/wiki/Direct_Rendering_Manager",
        tip: "DRM/KMS plane capture (Linux): grabs scanout buffers below the compositor. Very efficient, requires CAP_SYS_ADMIN.\nRuns the ffmpeg publish engine, the only one with a DRM/KMS source: GStreamer has no capture element for scanout buffers, only a kmssink.",
    },
    {
        value: "portal", label: "portal - PipeWire ScreenCast",
        link: "https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.ScreenCast.html",
        tip: "xdg-desktop-portal ScreenCast (Linux): the compositor's own picker chooses monitor or window, captured over PipeWire. Unprivileged and consented.\nIt is not Wayland's alone - the portal serves X11 sessions too, where the difference is the consent rather than the pixels: the X backends see the whole screen without asking, and the portal asks the compositor every time. Serving it is the compositor's, so a desktop with no ScreenCast backend fails the publish with the portal's own error.\nRuns the GStreamer publish engine, which reaches fewer pixel formats and rate-control knobs than ffmpeg; the fields say so where it matters.",
    },
    {
        value: "avfoundation", label: "avfoundation - AVFoundation screen capture",
        link: "https://developer.apple.com/documentation/avfoundation/avcapturescreeninput",
        tip: "AVFoundation screen capture (macOS): reads a screen device by index, cursor drawn in. Desktop audio is out of reach here, since AVFoundation enumerates input devices and what the machine plays is not one.\nRuns the ffmpeg publish engine.",
    },
    {
        value: "avfvideosrc", label: "avfvideosrc - AVFoundation screen capture (GStreamer)",
        link: "https://gstreamer.freedesktop.org/documentation/applemedia/avfvideosrc.html",
        tip: "AVFoundation screen capture (macOS), the same screen source as avfoundation read by GStreamer instead. The element picks the screen itself, so the monitor selection does not reach it.\nRuns the GStreamer publish engine: pick it over avfoundation to reach that engine's encoders on macOS.",
    },
];

// Where the captured frames reach the encoder. Values mirror the backend
// gpupath.Memories table.
//
// A capture backend that produces GPU frames and an encoder that reads GPU surfaces
// can be linked directly, and every other pair copies the picture to system memory,
// converts it on the CPU and, for a hardware encoder, uploads it again. Which pairs
// have the direct path is the backend's gpupath.Paths, which the form reads rather
// than restates: a value greyed here carries that table's own reason.
//
// Two of the values are that direct path and differ only in who converts, which the
// same table states per pair: a conversion on the device takes the colour selected, and
// a pair with no converter between its two ends leaves that to the encoder. The second
// is a value of its own so the trade is asked for rather than taken, which is why one
// of the two is always greyed once the pair is known.
export const FRAME_MEMORIES: Option[] = [
    {
        value: "auto", label: "auto - direct where possible",
        tip: "Keep the frames on the GPU where the capture backend and the encoder can share them, and copy them through system memory where they cannot. The one setting every combination satisfies.",
    },
    {
        value: "gpu", label: "gpu - stay on the device",
        link: "https://en.wikipedia.org/wiki/Direct_Rendering_Manager#DRM_PRIME",
        tip: "Hand the encoder the memory the capture already produced: no download, no CPU conversion, no upload. It demands the colour with it, so it is refused both where the selected capture backend and encoder have no shared path and where their shared path converts nothing on the way, rather than falling back.",
    },
    {
        value: "gpu-encoder-color", label: "gpu-encoder-color - device, encoder's colour",
        tip: "Hand the encoder the memory the capture already produced even where nothing on the way can convert it: no download and no CPU conversion, but the encoder reads the captured surface as it is and converts it itself, so the colour range and pixel format the stream carries are its choice rather than the ones selected - both fields grey with what it produces. Where the pair does convert on the device there is nothing to trade and this is the direct path itself; where the pair shares no memory at all it is refused, as gpu is.",
    },
    {
        value: "system", label: "system - copy to RAM",
        tip: "Download every frame, convert it on the CPU and upload it again for a hardware encoder. The path every combination has, and the one to pick when capture and encoding run on different GPUs.",
    },
];

// kmsgrab DRM download strategies. A scanout buffer is usually GPU tiled or
// compressed, so it maps to system memory through a hwdevice that understands
// the modifier. Which device works depends on the GPU; auto picks from the
// driver. Values mirror the backend DrmMaps table.
export const DRM_MAPS: Option[] = [
    {
        value: "auto", label: "auto - detect from GPU",
        tip: "Pick the mapping device from the capture GPU's kernel driver: VAAPI on Intel and AMD, Vulkan on NVIDIA and anything else.",
    },
    {
        value: "vaapi", label: "vaapi - Intel / AMD",
        link: "https://en.wikipedia.org/wiki/Video_Acceleration_API",
        tip: "Map the scanout buffer through VAAPI. Works where the driver exposes a VAAPI device (Intel, AMD).",
    },
    {
        value: "vulkan", label: "vulkan - cross-vendor",
        link: "https://en.wikipedia.org/wiki/Vulkan",
        tip: "Map through Vulkan, the cross-vendor DRM interop path. Use on NVIDIA, or when VAAPI is unavailable.",
    },
    {
        value: "none", label: "none - direct download",
        tip: "Download the frame with no mapping. Only correct for a linear (unmodified) framebuffer; a tiled or compressed scanout fails with EINVAL.",
    },
];

export const ENC_PRESETS: Option[] = [
    { value: "p1", label: "p1 - fastest", link: NVENC_LINK, tip: "Fastest ladder step: minimum lookahead and analysis, lowest compression efficiency." },
    { value: "p2", label: "p2", link: NVENC_LINK, tip: "Faster than default, slightly better compression than p1." },
    { value: "p3", label: "p3", link: NVENC_LINK, tip: "Balanced fast preset." },
    { value: "p4", label: "p4 - default", link: NVENC_LINK, tip: "NVENC default balance of speed and compression efficiency." },
    { value: "p5", label: "p5", link: NVENC_LINK, tip: "Slower, better compression. Used by latency mode with low-delay tuning." },
    { value: "p6", label: "p6", link: NVENC_LINK, tip: "Near-maximum analysis." },
    { value: "p7", label: "p7 - most efficient", link: NVENC_LINK, tip: "Maximum compression efficiency. On the dedicated encoder ASIC even p7 barely touches the 3D units." },
];

/** Metadata for transport values the backend registers, keyed by value. The text
 * describes the protocol itself, not one leg of the path, so the same entry serves
 * the publish dropdown and the watch dropdown. Anything true of only one leg
 * belongs in that field's own tooltip. */
export const TRANSPORT_META: Record<string, Option> = {
    srt: {
        value: "srt", label: "srt - Secure Reliable Transport",
        link: "https://en.wikipedia.org/wiki/Secure_Reliable_Transport",
        tip: "Secure Reliable Transport: UDP with selective retransmission (ARQ) and a configurable receive-window latency.\nThe caller opens one flow to the relay and media and control both ride it in either direction, so it asks no more of a NAT than any outbound connection does.",
    },
    rtsp: {
        value: "rtsp", label: "rtsp - Real-Time Streaming Protocol",
        link: "https://en.wikipedia.org/wiki/Real-Time_Streaming_Protocol",
        tip: "RTSP session carrying each track as its own RTP stream. Each leg picks the transport those streams run over: interleaved on the session's TCP connection, or a UDP port pair per track. Over TCP loss is handled by the connection, so there is no retransmit window to tune and delay rises on a lossy link instead.",
    },
    webrtc: {
        value: "webrtc", label: "webrtc - WHIP ingest, WHEP playback",
        link: "https://en.wikipedia.org/wiki/WebRTC",
        tip: "WebRTC through the relay's WHIP and WHEP endpoints: HTTP signaling, then SRTP. ICE runs connectivity checks before any media, and those checks are what opens the client's NAT, so this is the one transport that establishes its path rather than assuming it. What the publish leg carries follows the engine behind the capture backend, since the two WHIP sides negotiate different codec sets, and a codec one of them lacks is greyed with the reason. The watch leg is the native grid's and the browser's; no player opens it by URL, since WHEP is an exchange rather than an address.",
    },
    rtmp: {
        value: "rtmp", label: "rtmp - Real-Time Messaging Protocol",
        link: "https://en.wikipedia.org/wiki/Real-Time_Messaging_Protocol",
        tip: "FLV over one TCP connection, the protocol broadcast tools speak. Nothing about it is tunable: no retransmit window, no jitter buffer, and delay is whatever TCP and the relay's buffering make it.\nThe publish leg writes the enhanced-RTMP tags for H.265, AV1 and VP9 as well as H.264; the viewers here read H.264 alone back out of it.",
    },
    hls: {
        value: "hls", label: "hls - HTTP Live Streaming",
        link: "https://en.wikipedia.org/wiki/HTTP_Live_Streaming",
        tip: "Segments and a playlist over plain HTTP, which proxies and firewalls pass where the others are blocked. A viewer cannot start before a segment exists, so this is the slowest leg by a wide margin.\nWatch only: the relay serves HLS and ingests nothing over it.",
    },
};

/**
 * The RTP lower transport an RTSP leg runs over. Both legs choose from this one
 * list, the publish leg in the settings form and the watch leg beside each viewer,
 * so the text describes the protocol rather than a direction and what holds for
 * one leg alone belongs in that field's own tooltip.
 */
export const RTSP_PROTOCOLS: Option[] = [
    {
        value: "tcp", label: "tcp - interleaved over the RTSP connection",
        link: "https://en.wikipedia.org/wiki/Real-Time_Streaming_Protocol",
        tip: "Every track rides the one TCP connection the RTSP session already opened. Nothing is lost and no second port has to reach the far end, so nothing beyond the session itself has to cross a NAT or a filtering firewall. The cost is head-of-line blocking: a late packet holds up the frames queued behind it.",
    },
    {
        value: "udp", label: "udp - a port pair per track",
        link: "https://en.wikipedia.org/wiki/Real-time_Transport_Protocol",
        tip: "Each track negotiates its own UDP port pair, separate from the RTSP connection, which drops the delay TCP's in-order delivery adds. Lost RTP is never retransmitted, so loss shows as artifacts rather than as delay.\nBehind a home NAT that pair is reached by sending from it first, which creates the mapping anything coming back needs: the publish leg does that with the media itself, the watch leg with probe packets, and the relay then has to answer where those arrived rather than at the port the session announced, which is the private one the NAT rewrote. A network that drops outbound UDP ends it either way, and the failure is silent: the session sets up and no frame follows.",
    },
];

/**
 * Tooltip for the quantizer target. Every encoder has this control under its own
 * name, and the scale follows the encoder and the engine that drives it: the H.26x
 * ones stop at 51 where libvpx and the software AV1 ones count to 63, and one taking
 * a raw quantizer index reaches 127 or 255. The quality landmarks are therefore
 * placed on the running combination's own scale rather than quoted from x264's, and
 * a combination that declares none carries the description alone.
 */
export function cqTip(
    codec: string,
    engine: Engine | null,
    caps: Capability[] | null
): string {
    const base =
        "Constant quantizer the encoder holds in constant-quality mode: x264 and libvpx call it CRF, x265 QP, NVENC CQ. Lower = better quality and more bits.";
    const max = cqMax(codec, engine, caps);
    if (max <= 0) {
        return base;
    }
    const at = (onFiftyOne: number) => scaleCq(onFiftyOne, codec, engine, caps);
    return (
        `${base}\n` +
        `This codec's scale runs 0-${max}: ${at(12)} ≈ visually lossless, ${at(19)} ≈ excellent, ${at(28)} ≈ visibly compressed.`
    );
}

/**
 * Tooltip for the bitrate target, which names the codec's ceiling on the running
 * engine where it has one. An encoder that refuses a target above its own limit is
 * worth saying so before the user types a number the publish would die on.
 */
export function bitrateTip(
    codec: string,
    engine: Engine | null,
    caps: Capability[] | null
): string {
    const base =
        "Target rate for CBR (held constant), VBR and ABR (averaged toward).";
    const limit = bitrateLimit(codec, engine, caps);
    return limit > 0
        ? `${base}\nThis encoder takes at most ${limit} Mbit/s and refuses anything above it.`
        : base;
}

/**
 * Appends a dependency note to a field tooltip. A note carries what the value
 * does in a combination the base text does not cover, so the base text can stay
 * the general description of the control.
 */
export function withNote(tip: string, note?: string): string {
    return note ? `${tip}\n${note}` : tip;
}

/** Capture backends that crop the X screen to the selected monitor's geometry
 * and can only do so when enumeration reported one. */
const X_CROPPING_CAPTURES = ["x11grab", "ximagesrc"];

/**
 * Why the monitor selection is not in force, empty when it is.
 *
 * The X backends crop by offset and size, so a monitor enumeration reported no
 * geometry for leaves them nothing to crop to and they capture the whole X
 * screen instead. That is a different picture from the one the field names: at
 * multi-monitor width it can exceed an encoder's dimension limit, and it is
 * never what was asked for. The capture still runs, so this is a note rather
 * than a refusal, but it has to be said.
 */
export function cropNote(
    capture: string,
    monitor: { width: number; height: number } | undefined
): string {
    if (!X_CROPPING_CAPTURES.includes(capture)) return "";
    if (monitor && monitor.width > 0 && monitor.height > 0) return "";
    return `Not in force: monitor enumeration reported no geometry, so ${capture} captures the whole X screen rather than one monitor. Install the monitor enumerator for this session (xrandr on X11) to crop.`;
}

/** Returns the display label for value, falling back to the raw value. */
export function labelFor(options: Option[], value: string): string {
    return options.find(o => o.value === value)?.label ?? value;
}

/** Common capture/encode frame rates offered as dropdown prefills. */
const FPS_PRESETS = [30, 60, 90, 120, 144, 165, 240];

/**
 * Builds one option per preset frame rate. Always includes the saved value so a
 * custom rate outside the preset ladder stays selected and visible.
 */
export function fpsOptions(current: number): Option[] {
    const values = FPS_PRESETS.includes(current)
        ? FPS_PRESETS
        : [...FPS_PRESETS, current].sort((a, b) => a - b);
    return values.map(fps => ({
        value: String(fps),
        label: `${fps} fps`,
    }));
}

/**
 * The highest refresh rate any detected monitor reports, 0 when none reports
 * one. This is the ceiling the frame rate field measures against, rather than
 * the selected monitor's rate: which monitor is selected is a separate choice
 * that changes as often as the capture target does, and tying the ladder to it
 * puts the rates of a faster monitor out of reach until the selection moves.
 */
export function maxRefreshHz(monitors: Monitor[]): number {
    return monitors.reduce((hz, m) => Math.max(hz, m.refreshHz), 0);
}

/**
 * Reasons for frame rates no monitor can display, keyed by fps. Capturing above
 * the fastest monitor's refresh rate yields duplicate frames whichever monitor
 * is captured, so those rates are shown but disabled. maxHz of 0 (unknown
 * refresh, or no monitor) disables nothing. The saved value is never disabled so
 * it stays selectable.
 */
export function fpsDisabled(current: number, maxHz: number): Record<string, string> {
    const reasons: Record<string, string> = {};
    if (maxHz > 0) {
        for (const fps of FPS_PRESETS) {
            if (fps > maxHz && fps !== current) {
                reasons[String(fps)] = `Above the fastest monitor's ${maxHz} Hz refresh rate.`;
            }
        }
    }
    return reasons;
}

/**
 * Builds one option per detected monitor, labeling each with its resolution and
 * primary flag. Always includes the saved index so a stale selection stays
 * visible even if that monitor is gone.
 */
export function monitorOptions(monitors: Monitor[], current: number): Option[] {
    const options = monitors.map(m => {
        const dims = m.width && m.height ? `${m.width}×${m.height}` : "";
        const hz = m.refreshHz ? `${m.refreshHz} Hz` : "";
        const spec = [dims, hz].filter(Boolean).join(", ");
        const res = spec ? ` (${spec})` : "";
        const primary = m.primary ? ", primary" : "";
        return {
            value: String(m.index),
            label: `Monitor ${m.index}${res}`,
            tip: `Capture index ${m.index}${primary}.`,
        };
    });

    if (!options.some(o => o.value === String(current))) {
        options.push({
            value: String(current),
            label: `Monitor ${current}`,
            tip: `Capture index ${current}.`,
        });
    }
    return options;
}
