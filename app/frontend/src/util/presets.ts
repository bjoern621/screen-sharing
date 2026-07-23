import { Stream } from "../types/stream";

/**
 * Master presets. Selecting one applies every field below at once; changing any
 * field afterwards flips the preset selector back to "custom".
 */
export const PRESETS: Record<string, Partial<Stream>> = {
    "lossless-rgb": {
        // LAN/localhost: loss is ~0, so the SRT windows only need to absorb
        // scheduling jitter. ~180 ms total instead of a WAN-sized 1.5 s.
        mode: "lossless", codec: "hevc_nvenc", chroma: "gbrp", encPreset: "p7",
        bframes: 0, fps: 60, gop: 0, srtPublishLatencyMs: 60, srtWatchLatencyMs: 120,
    },
    "near-lossless-rgb": {
        // Fast link (heavy ~200 Mbit RGB), still not the open internet: a
        // moderate ~650 ms of retransmit room.
        mode: "quality", codec: "hevc_nvenc", chroma: "gbrp", encPreset: "p7",
        bframes: 0, fps: 60, gop: 0, cq: 12, bitrateM: 200,
        srtPublishLatencyMs: 150, srtWatchLatencyMs: 500,
    },
    "quality-444": {
        mode: "quality", codec: "hevc_nvenc", chroma: "yuv444p", encPreset: "p7",
        bframes: 0, fps: 60, gop: 0, cq: 16, bitrateM: 150, colorRange: "pc",
        srtPublishLatencyMs: 300, srtWatchLatencyMs: 1200,
    },
    "bandwidth-420": {
        mode: "quality", codec: "hevc_nvenc", chroma: "yuv420p", encPreset: "p7",
        bframes: 2, fps: 60, gop: 0, cq: 23, bitrateM: 40, colorRange: "pc",
        srtPublishLatencyMs: 300, srtWatchLatencyMs: 1200,
    },
    "low-latency": {
        mode: "latency", codec: "hevc_nvenc", chroma: "yuv420p", encPreset: "p5",
        bframes: 0, fps: 60, gop: 0, bitrateM: 20, colorRange: "pc",
        srtPublishLatencyMs: 120, srtWatchLatencyMs: 250,
    },
    "web-viewable": {
        mode: "quality", codec: "h264_nvenc", chroma: "yuv420p", encPreset: "p7",
        bframes: 2, fps: 60, gop: 0, cq: 23, bitrateM: 40, colorRange: "tv",
        srtPublishLatencyMs: 300, srtWatchLatencyMs: 1200,
    },
};

/** Display label for each preset, including the synthetic "custom" state. */
export const PRESET_LABELS: Record<string, string> = {
    custom: "Custom",
    "lossless-rgb": "Mathematically lossless RGB (LAN)",
    "near-lossless-rgb": "Near-lossless RGB (CQ 12)",
    "quality-444": "High quality Y′CbCr 4:4:4",
    "bandwidth-420": "Bandwidth saver 4:2:0",
    "low-latency": "Low latency (CBR)",
    "web-viewable": "Web-viewable (H.264 4:2:0)",
};

/** One-line description shown beside the selected preset. */
export const PRESET_HINTS: Record<string, string> = {
    "lossless-rgb": "bit-exact desktop pixels, bursts to 100s of Mbps on motion - LAN/localhost only",
    "near-lossless-rgb": "RGB cq12, visually identical to lossless at a fraction of the bytes",
    "quality-444": "crisp text/color, remote-friendly",
    "bandwidth-420": "chroma subsampled like Discord/Twitch - smallest, softest color edges",
    "low-latency": "CBR + short SRT windows (~0.4s total), quality sacrificed for delay",
    "web-viewable": "H.264 4:2:0 limited range - every browser can decode it via the relay HLS page, no app needed",
    custom: "",
};
