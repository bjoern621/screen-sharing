import { Deps, PlatformInfo, Stream } from "../types/stream";

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
        };
    }
    if (platform.os === "linux") {
        const out: Record<string, string> = {
            ddagrab: "DXGI Desktop Duplication is Windows-only",
            gdigrab: "GDI capture is Windows-only",
        };
        if (platform.display === "wayland") {
            out.x11grab =
                "Wayland session: x11grab only sees XWayland windows, not the Wayland desktop - use kmsgrab";
        }
        return out;
    }
    return {};
}

/** The capture API to fall back to when the current one is unavailable here. */
function preferredCapture(platform: PlatformInfo | null): string {
    if (platform?.os === "linux") {
        return platform.display === "wayland" ? "kmsgrab" : "x11grab";
    }
    return "ddagrab";
}

/**
 * Evaluates which controls and individual options are unavailable for the given
 * settings and platform, mirroring the constraints the encoder/relay enforce in
 * ffmpeg/args.go. Pure: no React, no side effects.
 */
export function evaluateDeps(s: Stream, platform: PlatformInfo | null): Deps {
    const isNvenc = s.codec.endsWith("_nvenc");
    const d: Deps = {
        disabled: {},
        optionDisabled: {
            codec: {},
            chroma: {},
            capture: unavailableCaptures(platform),
        },
    };

    if (s.transport === "srt") {
        d.optionDisabled.codec["av1_nvenc"] =
            "MediaMTX SRT/MPEG-TS ingest carries H.264/H.265 only";
    }
    if (s.codec !== "hevc_nvenc") {
        d.optionDisabled.chroma["gbrp"] =
            "direct RGB coding needs HEVC Range Extensions (hevc_nvenc)";
    }
    if (s.codec === "av1_nvenc") {
        d.optionDisabled.chroma["yuv444p"] = "NVENC AV1 encodes 4:2:0 only";
    }

    if (s.mode !== "quality") {
        d.disabled.cq = "the quantizer target only exists in quality mode";
    }
    if (s.mode === "lossless") {
        d.disabled.bitrateM =
            "lossless output costs whatever exactness costs - no bitrate bound";
    }
    if (s.mode !== "quality" || !isNvenc) {
        d.disabled.bframes =
            "B-frames are forced off in lossless/latency modes (no gain, only reorder delay)";
    }
    if (!isNvenc || s.mode === "latency") {
        d.disabled.encPreset = isNvenc
            ? "latency mode pins the preset to p5"
            : "the p1-p7 ladder is NVENC-specific";
    }
    if (s.chroma === "gbrp") {
        d.disabled.colorRange =
            "RGB is inherently full range - no quantization range choice exists";
    }
    if (s.capture !== "ddagrab") {
        d.disabled.monitor = "only DXGI Desktop Duplication captures per monitor";
    }

    return d;
}

/**
 * Applies availability fallbacks so settings never hold a combination the relay,
 * encoder or platform would reject. Returns a new object; never mutates the
 * input. A null platform leaves the capture API untouched.
 */
export function normalize(s: Stream, platform: PlatformInfo | null = null): Stream {
    const next = { ...s };
    if (next.transport === "srt" && next.codec === "av1_nvenc") {
        next.codec = "hevc_nvenc";
    }
    if (next.codec !== "hevc_nvenc" && next.chroma === "gbrp") {
        next.chroma = "yuv444p";
    }
    if (next.codec === "av1_nvenc" && next.chroma === "yuv444p") {
        next.chroma = "yuv420p";
    }
    if (unavailableCaptures(platform)[next.capture]) {
        next.capture = preferredCapture(platform);
    }
    return next;
}
