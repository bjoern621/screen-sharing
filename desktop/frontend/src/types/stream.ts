import { settings, relay, display, platform, encoders, gpupath, main } from "../../wailsjs/go/models";

/** Wire-format stream settings, shared verbatim with the Go backend. */
export type Stream = settings.Stream;

/** A user-saved, named snapshot of stream settings. */
export type Preset = settings.Preset;

/** Relay discovery snapshot (reachability + per-path live figures). */
export type RelayStatus = relay.Status;

/** What one engine carries over one leg of one transport: the video bitstream
 * formats and the audio codec formats. The table holds a row per (transport, leg,
 * engine) triple, and a slot with no row is an engine that cannot serialize that
 * leg of that protocol, so a publish rule and a watch verdict each read their own
 * rows rather than halves of a shared one. */
export type TransportCarriage = main.TransportCarriage;

/** One display output: capture index, resolution and primary flag. */
export type Monitor = display.Monitor;

/** One capture backend and encoder family whose frames reach the encoder without a
 * trip through system memory, with the import that carries them. */
export type GpuPath = gpupath.Path;

/** Running platform: OS and, on Linux, the display server (x11/wayland). */
export type PlatformInfo = platform.Info;

/**
 * Which video encoders this machine can run, probed at startup: publish engine ->
 * codec -> whether it ran. An engine or codec the probe left out imposes no
 * restriction. The generated binding types the nested map as `any` (Go's
 * map[string]map[string]bool), so the shape is restated here.
 */
export interface EncoderInfo extends encoders.Availability {
    usable: Record<string, Record<string, boolean>>;
    unprobed: Record<string, string>;
}

/**
 * One encoder progress sample, emitted by whichever publish engine runs the
 * session. The engines agree on what each figure means (see `ffmpeg.Stats` in
 * proc.go): `fps`, `captureFps` and `instMbps` are rates over the interval since
 * the previous sample, `speed` and `avgMbps` are cumulative over the run, `dup`
 * counts frames the encoder repeated to hold the output rate and `drop` counts
 * input frames it discarded for arriving faster than that rate.
 *
 * A null is the engine reporting no measurement for that figure, which a stalled
 * encoder's measured zero is not. Every figure an engine can be without a value
 * for is nullable, so the renderer has to decide what a missing one looks like.
 */
export interface Stats {
    frame: number;
    fps: number | null;
    captureFps: number | null;
    sizeKiB: number | null;
    timeSec: number | null;
    speed: number | null;
    dup: number;
    drop: number;
    instMbps: number | null;
    avgMbps: number | null;
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

/** Payload of the "publish:state" event: whether the app is publishing. It
 * reports every change, including the ones this window did not make. */
export interface PublishState {
    publishing: boolean;
}

/** Payload of the "watch:exit" event. name and transport together identify
 * which viewer exited, since one stream can be watched over several transports. */
export interface WatchExit {
    name: string;
    transport: string;
    message: string;
    logPath: string;
}
