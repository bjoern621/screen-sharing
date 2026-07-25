import { settings, relay, display, platform, encoders } from "../../wailsjs/go/models";

/** Wire-format stream settings, shared verbatim with the Go backend. */
export type Stream = settings.Stream;

/** A user-saved, named snapshot of stream settings. */
export type Preset = settings.Preset;

/** Relay discovery snapshot (reachability + per-path live figures). */
export type RelayStatus = relay.Status;

/** One display output: capture index, resolution and primary flag. */
export type Monitor = display.Monitor;

/** Running platform: OS and, on Linux, the display server (x11/wayland). */
export type PlatformInfo = platform.Info;

/** Which hardware video encoders this machine can run, probed at startup. */
export type EncoderInfo = encoders.Availability;

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

/** A selectable option with an optional explanatory tooltip and reference link. */
export interface Option {
    value: string;
    label: string;
    tip?: string;
    link?: string;
}

/**
 * Outcome of dependency evaluation: which controls and which individual options
 * are unavailable for the current settings, each with a human-readable reason.
 */
export interface Deps {
    /** control id -> reason the whole control is ignored/disabled */
    disabled: Record<string, string>;
    /** control id -> what the value does in a combination that gives the control a
     * meaning its own tooltip does not cover, appended to that tooltip. A control
     * carrying a note is live: the note explains, it does not block. */
    note: Record<string, string>;
    /** control id -> option value -> reason that single option is unavailable */
    optionDisabled: Record<string, Record<string, string>>;
}

/** Whether one viewer can show the configured stream, with a reason either way.
 * The settings form carries one verdict per grid: web and native. */
export interface ViewVerdict {
    ok: boolean;
    text: string;
}

/** Payload of the "publish:exit" event. */
export interface PublishExit {
    message: string;
    logPath: string;
}

/** Payload of the "watch:exit" event. name and transport together identify
 * which viewer exited, since one stream can be watched over several transports. */
export interface WatchExit {
    name: string;
    transport: string;
    message: string;
    logPath: string;
}
