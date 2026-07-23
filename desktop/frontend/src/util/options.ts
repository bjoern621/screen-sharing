import { Monitor, Option } from "../types/stream";
import { CHROMA_META, CODEC_META, MODE_META, metaOptions } from "./domain";

const NVENC_LINK = "https://en.wikipedia.org/wiki/Nvidia_NVENC";

// Codec, mode and chroma option lists are derived from the domain meta tables so
// the dropdowns, the dependency rules and the heuristics share one definition.
export const CODECS: Option[] = metaOptions(CODEC_META);
export const MODES: Option[] = metaOptions(MODE_META);
export const CHROMAS: Option[] = metaOptions(CHROMA_META);

export const RANGES: Option[] = [
    {
        value: "pc", label: "pc - full range (0–255)",
        link: "https://en.wikipedia.org/wiki/YCbCr",
        tip: "Full range: all code values carry image data. Correct for computer graphics; mismatch causes crushed or washed tones.",
    },
    {
        value: "tv", label: "tv - limited / studio swing (16–235)",
        link: "https://en.wikipedia.org/wiki/Broadcast-safe",
        tip: "Limited/studio swing, a broadcast legacy. Only pick when a downstream device demands it (or for maximum web-player compatibility).",
    },
];

export const CAPTURES: Option[] = [
    {
        value: "ddagrab", label: "ddagrab - DXGI Desktop Duplication",
        link: "https://learn.microsoft.com/en-us/windows/win32/direct3ddxgi/desktop-dup-api",
        tip: "DXGI Desktop Duplication (Windows): captures the composited framebuffer on the GPU, per monitor. Preferred on Windows.",
    },
    {
        value: "gdigrab", label: "gdigrab - GDI BitBlt",
        link: "https://learn.microsoft.com/en-us/windows/win32/api/wingdi/nf-wingdi-bitblt",
        tip: "GDI BitBlt (Windows): CPU copy of the whole desktop - all monitors as ONE frame; multi-monitor widths can exceed NVENC's 8192 px limit.",
    },
    {
        value: "x11grab", label: "x11grab - X11 SHM",
        link: "https://ffmpeg.org/ffmpeg-devices.html#x11grab",
        tip: "X11 shared-memory capture (Linux, also XWayland windows). Default on Linux; pure-Wayland surfaces need a portal-based path instead.",
    },
    {
        value: "kmsgrab", label: "kmsgrab - DRM/KMS",
        link: "https://en.wikipedia.org/wiki/Direct_Rendering_Manager",
        tip: "DRM/KMS plane capture (Linux): grabs scanout buffers below the compositor. Very efficient, requires CAP_SYS_ADMIN.",
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

/** Metadata for transport values the backend registers, keyed by value. */
export const TRANSPORT_META: Record<string, Option> = {
    srt: {
        value: "srt", label: "srt - Secure Reliable Transport",
        link: "https://en.wikipedia.org/wiki/Secure_Reliable_Transport",
        tip: "Secure Reliable Transport: UDP with selective retransmission (ARQ) and a configurable receive-window latency.",
    },
};

/** Returns the display label for value, falling back to the raw value. */
export function labelFor(options: Option[], value: string): string {
    return options.find(o => o.value === value)?.label ?? value;
}

/**
 * Builds one option per detected monitor, labeling each with its resolution and
 * primary flag. Always includes the saved index so a stale selection stays
 * visible even if that monitor is gone.
 */
export function monitorOptions(monitors: Monitor[], current: number): Option[] {
    const options = monitors.map(m => {
        const res = m.width && m.height ? ` (${m.width}×${m.height})` : "";
        const primary = m.primary ? ", primary" : "";
        return {
            value: String(m.index),
            label: `Monitor ${m.index}${res}`,
            tip: `DXGI output index ${m.index}${primary}. ddagrab captures one monitor per index.`,
        };
    });

    if (!options.some(o => o.value === String(current))) {
        options.push({
            value: String(current),
            label: `Monitor ${current}`,
            tip: `DXGI output index ${current}.`,
        });
    }
    return options;
}
