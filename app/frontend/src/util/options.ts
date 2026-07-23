import { Monitor, Option } from "../types/stream";

const NVENC_LINK = "https://en.wikipedia.org/wiki/Nvidia_NVENC";

export const CODECS: Option[] = [
    {
        value: "hevc_nvenc", label: "HEVC / H.265 - NVENC hardware",
        link: "https://en.wikipedia.org/wiki/High_Efficiency_Video_Coding",
        tip: "High Efficiency Video Coding (ITU-T H.265 | ISO/IEC 23008-2) on NVIDIA's encoder ASIC. Only NVENC codec with 4:4:4/RGB support here.",
    },
    {
        value: "h264_nvenc", label: "AVC / H.264 - NVENC hardware",
        link: "https://en.wikipedia.org/wiki/Advanced_Video_Coding",
        tip: "Advanced Video Coding (ITU-T H.264 | ISO/IEC 14496-10) on NVIDIA's encoder ASIC. Widest decoder compatibility, less efficient than HEVC.",
    },
    {
        value: "av1_nvenc", label: "AV1 - NVENC hardware (4:2:0 only)",
        link: "https://en.wikipedia.org/wiki/AV1",
        tip: "AOMedia Video 1 on NVIDIA's encoder ASIC (RTX 40+). Most efficient per bit, but NVENC AV1 encodes 4:2:0 only.",
    },
    {
        value: "libx264", label: "AVC / H.264 - x264 software",
        link: "https://en.wikipedia.org/wiki/X264",
        tip: "AVC/H.264 in software (x264). CPU-heavy at high resolutions; fallback when no capable GPU encoder exists.",
    },
];

export const MODES: Option[] = [
    {
        value: "lossless", label: "lossless - bit-exact",
        link: "https://en.wikipedia.org/wiki/Lossless_compression",
        tip: "Mathematically lossless: decoded output is bit-identical to input. Bitrate unbounded - bursts to hundreds of Mbit/s on motion. LAN only.",
    },
    {
        value: "quality", label: "quality - VBR + constant quantizer",
        link: "https://en.wikipedia.org/wiki/Variable_bitrate",
        tip: "Variable bitrate targeting a constant quantizer (CQ): quality held constant, bitrate varies with content. The bitrate bound only caps bursts.",
    },
    {
        value: "latency", label: "latency - CBR low-delay",
        link: "https://en.wikipedia.org/wiki/Constant_bitrate",
        tip: "Constant bitrate with low-delay tuning: fixed bandwidth, quality varies, smallest buffers and delay.",
    },
];

export const CHROMAS: Option[] = [
    {
        value: "gbrp", label: "gbrp - planar RGB, no subsampling",
        link: "https://en.wikipedia.org/wiki/RGB_color_model",
        tip: "Planar RGB coded directly via HEVC Range Extensions (identity matrix). Zero color conversion - exact desktop pixels. Heaviest option.",
    },
    {
        value: "yuv444p", label: "yuv444p - Y′CbCr 4:4:4",
        link: "https://en.wikipedia.org/wiki/Chroma_subsampling",
        tip: "Y′CbCr with full-resolution chroma (4:4:4). Near-indistinguishable from RGB after correct conversion; slightly cheaper than gbrp.",
    },
    {
        value: "yuv420p", label: "yuv420p - Y′CbCr 4:2:0",
        link: "https://en.wikipedia.org/wiki/Chroma_subsampling",
        tip: "Chroma at quarter resolution (4:2:0). Smallest and universally decodable - colored text/edges smear: the washed-out Discord/WebRTC look.",
    },
    {
        value: "p010le", label: "p010le - 10-bit Y′CbCr 4:2:0",
        link: "https://en.wikipedia.org/wiki/Color_depth",
        tip: "10-bit Y′CbCr 4:2:0. More tonal resolution (HDR), still chroma-subsampled. Only useful for >8-bit sources.",
    },
];

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
