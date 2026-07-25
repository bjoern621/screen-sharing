import { useCallback, useEffect, useMemo, useState } from "react";
import {
    GetSettings, SaveSettings, GetPresets, SavePreset, DeletePreset,
    PublishCommand, Transports, CaptureTransports, CaptureEngines,
} from "../../wailsjs/go/main/App";
import {
    Deps, EncoderInfo, PlatformInfo, Preset, Stream, ViewVerdict,
} from "../types/stream";
import { Environment, evaluateDeps, normalize } from "../util/deps";
import { Capability, Engine, engineFor } from "../util/domain";
import { nativeGridCheck } from "../util/nativegrid";
import { webGridCheck } from "../util/webgrid";
import { PRESETS } from "../util/presets";

export const CUSTOM_PRESET = "custom";

// User presets share the dropdown with the built-in ones, so their selector
// values are namespaced to never collide with a built-in key or "custom".
export const USER_PREFIX = "user:";

/** Selector value for a saved preset. */
export function userPresetValue(name: string): string {
    return USER_PREFIX + name;
}

/** True field-by-field equality of two settings snapshots. */
function streamEquals(a: Stream, b: Stream): boolean {
    const keys = Object.keys(a) as (keyof Stream)[];
    return keys.every(k => a[k] === b[k]);
}

/**
 * Which selector value the given settings correspond to: an exact user preset,
 * else a built-in preset whose every field already matches, else "custom".
 */
function matchPreset(s: Stream, userPresets: Preset[]): string {
    const user = userPresets.find(p => streamEquals(s, p.settings));
    if (user) {
        return userPresetValue(user.name);
    }
    const builtin = Object.entries(PRESETS).find(([, partial]) =>
        Object.entries(partial).every(([k, v]) => s[k as keyof Stream] === v)
    );
    return builtin ? builtin[0] : CUSTOM_PRESET;
}

/**
 * Owns the editable stream settings and everything derived from them: the
 * dependency map, the web- and native-grid verdicts, live command preview and the
 * transport list. Any field change re-normalizes the settings and drops the
 * preset back to "custom"; applying a preset patches many fields at once without
 * doing so.
 * The platform gates which capture backends are available, the encoder set which
 * codecs the machine can run, the capability table which codec/chroma/transport
 * combinations are legal, and the capture backend's engine which rate-control
 * knobs reach the encoder.
 *
 * The working settings are persisted on every change, so the next launch
 * restores the exact last state whether or not it was saved as a named preset.
 * Named presets are stored separately and can be saved and deleted at will.
 */
export function useStreamSettings(
    platform: PlatformInfo | null,
    encoders: EncoderInfo | null,
    caps: Capability[] | null
) {
    const [s, setS] = useState<Stream | null>(null);
    const [preset, setPreset] = useState(CUSTOM_PRESET);
    const [userPresets, setUserPresets] = useState<Preset[]>([]);
    const [transports, setTransports] = useState<string[]>(["srt"]);
    const [captureTransports, setCaptureTransports] =
        useState<Record<string, string[]> | null>(null);
    const [captureEngines, setCaptureEngines] =
        useState<Record<string, string> | null>(null);
    const [cmd, setCmd] = useState("");

    // One value for every fact the dependency rules read, so the evaluation and
    // the repairs cannot be handed different subsets.
    const env: Environment = useMemo(
        () => ({ platform, encoders, caps, captureTransports, captureEngines }),
        [platform, encoders, caps, captureTransports, captureEngines]
    );

    const deps: Deps | null = useMemo(
        () => (s ? evaluateDeps(s, env) : null),
        [s, env]
    );
    const webGrid: ViewVerdict | null = useMemo(
        () => (s ? webGridCheck(s, caps) : null),
        [s, caps]
    );
    const nativeGrid: ViewVerdict | null = useMemo(
        () => (s ? nativeGridCheck(s, caps) : null),
        [s, caps]
    );
    // The publish engine the selected capture backend runs on. Derived here rather
    // than in the form, so the form and the dependency rules read one value.
    const engine: Engine | null = useMemo(
        () => (s ? engineFor(s.capture, captureEngines) : null),
        [s, captureEngines]
    );

    const update = useCallback(
        (patch: Partial<Stream>, fromPreset = false) => {
            setS(prev =>
                prev ? normalize({ ...prev, ...patch } as Stream, env) : prev
            );
            if (!fromPreset) {
                setPreset(CUSTOM_PRESET);
            }
        },
        [env]
    );

    const applyPreset = useCallback(
        (name: string) => {
            setPreset(name);
            if (name.startsWith(USER_PREFIX)) {
                const p = userPresets.find(p => userPresetValue(p.name) === name);
                if (p) {
                    update(p.settings, true);
                }
                return;
            }
            const p = PRESETS[name];
            if (p) {
                update(p, true);
            }
        },
        [update, userPresets]
    );

    const saveAsPreset = useCallback(
        async (name: string) => {
            const trimmed = name.trim();
            if (!s || !trimmed) {
                return;
            }
            await SavePreset(trimmed, s);
            setUserPresets(await GetPresets());
            setPreset(userPresetValue(trimmed));
        },
        [s]
    );

    const deletePreset = useCallback(async (value: string) => {
        if (!value.startsWith(USER_PREFIX)) {
            return;
        }
        await DeletePreset(value.slice(USER_PREFIX.length));
        setUserPresets(await GetPresets());
        setPreset(prev => (prev === value ? CUSTOM_PRESET : prev));
    }, []);

    useEffect(() => {
        void (async () => {
            const loaded = normalize(await GetSettings());
            const presets = await GetPresets();
            setUserPresets(presets);
            setS(loaded);
            setPreset(matchPreset(loaded, presets));
            setTransports(await Transports());
            setCaptureTransports(await CaptureTransports());
            setCaptureEngines(await CaptureEngines());
        })();
    }, []);

    // Re-normalize whenever a dimension resolves after mount: platform gates the
    // capture backend (ddagrab on Linux falls back), the encoder probe and capability
    // table gate the codec/chroma (hevc_nvenc drops to x264 without an NVIDIA
    // encoder), the capture->transport map gates the transport (the GStreamer engine
    // drops WebRTC), and the capture->engine map gates the chroma (the portal
    // path's encoders drop planar RGB). Any illegal carryover from the persisted
    // settings is repaired to a valid combination.
    useEffect(() => {
        if (platform || encoders || caps || captureTransports || captureEngines) {
            setS(prev => (prev ? normalize(prev, env) : prev));
        }
    }, [platform, encoders, caps, captureTransports, captureEngines, env]);

    useEffect(() => {
        if (!s) {
            return;
        }
        PublishCommand(s)
            .then(setCmd)
            .catch(e => setCmd(String(e)));
    }, [s]);

    // Persist the working settings so the next launch restores them. Debounced
    // to coalesce keystrokes into one write; the pagehide flush covers a quit
    // inside the debounce window.
    useEffect(() => {
        if (!s) {
            return;
        }
        const flush = () => void SaveSettings(s);
        const id = setTimeout(flush, 400);
        window.addEventListener("pagehide", flush);
        return () => {
            clearTimeout(id);
            window.removeEventListener("pagehide", flush);
        };
    }, [s]);

    return {
        s, preset, userPresets, transports, engine, deps, webGrid, nativeGrid, cmd,
        update, applyPreset, saveAsPreset, deletePreset,
    };
}
