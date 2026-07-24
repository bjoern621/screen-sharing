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
    {
        value: "portal", label: "portal - PipeWire ScreenCast",
        link: "https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.ScreenCast.html",
        tip: "xdg-desktop-portal ScreenCast (Wayland): the compositor's own picker chooses monitor/window, captured over PipeWire. Unprivileged and consented; runs the GStreamer pipeline.",
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
 * Reasons for frame rates the selected monitor cannot display, keyed by fps.
 * Capturing above the monitor's refresh rate yields duplicate frames, so those
 * rates are shown but disabled. maxHz of 0 (unknown refresh, or no monitor)
 * disables nothing. The saved value is never disabled so it stays selectable.
 */
export function fpsDisabled(current: number, maxHz: number): Record<string, string> {
    const reasons: Record<string, string> = {};
    if (maxHz > 0) {
        for (const fps of FPS_PRESETS) {
            if (fps > maxHz && fps !== current) {
                reasons[String(fps)] = `Above the monitor's ${maxHz} Hz refresh rate.`;
            }
        }
    }
    return reasons;
}

/**
 * Clamps a frame rate to what a monitor can display. Returns fps unchanged when
 * the refresh rate is unknown (maxHz 0) or already high enough; otherwise the
 * monitor's refresh rate, the fastest rate it can actually show.
 */
export function clampFps(fps: number, maxHz: number): number {
    return maxHz > 0 && fps > maxHz ? maxHz : fps;
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
