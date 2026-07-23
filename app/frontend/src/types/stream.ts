import { settings, relay, display, platform } from "../../wailsjs/go/models";

/** Wire-format stream settings, shared verbatim with the Go backend. */
export type Stream = settings.Stream;

/** Relay discovery snapshot (reachability + per-path live figures). */
export type RelayStatus = relay.Status;

/** One display output: capture index, resolution and primary flag. */
export type Monitor = display.Monitor;

/** Running platform: OS and, on Linux, the display server (x11/wayland). */
export type PlatformInfo = platform.Info;

/**
 * One encoder progress sample. The backend derives the instantaneous bitrate
 * (Δbytes/Δtime); the frontend keeps ffmpeg's cumulative average alongside it.
 */
export interface Stats {
    frame: number;
    fps: number;
    sizeKiB: number;
    timeSec: number;
    speed: number;
    drop: number;
    instMbps: number;
    avgMbps: number;
}

/** A selectable option with an explanatory tooltip and optional reference link. */
export interface Option {
    value: string;
    label: string;
    tip: string;
    link?: string;
}

/**
 * Outcome of dependency evaluation: which controls and which individual options
 * are unavailable for the current settings, each with a human-readable reason.
 */
export interface Deps {
    /** control id -> reason the whole control is ignored/disabled */
    disabled: Record<string, string>;
    /** control id -> option value -> reason that single option is unavailable */
    optionDisabled: Record<string, Record<string, string>>;
}

/** Verdict of the browser-viewability check shown under the publish controls. */
export interface BrowserVerdict {
    ok: boolean;
    text: string;
}

/** Payload of the "publish:exit" event. */
export interface PublishExit {
    message: string;
    logPath: string;
}

/** Payload of the "watch:exit" event. */
export interface WatchExit {
    name: string;
    message: string;
    logPath: string;
}
