import { useCallback, useEffect, useMemo, useState } from "react";
import {
    GetSettings, SaveSettings, GetPresets, SavePreset, DeletePreset,
    PublishCommand, Transports, TransportFormats, CaptureTransports,
    CaptureEngines, StoreNotice,
} from "../../wailsjs/go/main/App";
import {
    Deps, EncoderInfo, PlatformInfo, Preset, Stream, TransportCarriage,
    ViewVerdict,
} from "../types/stream";
import { Environment, evaluateDeps, normalize } from "../util/deps";
import { Capability, Decoder, Engine, engineFor } from "../util/domain";
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
 * knobs reach the encoder. The decode table restricts nothing: it describes the
 * viewers rather than this machine, so it only lets the form say what a pixel format
 * costs them.
 *
 * The working settings are persisted on every change, so the next launch
 * restores the exact last state whether or not it was saved as a named preset.
 * Named presets are stored separately and can be saved and deleted at will.
 */
export function useStreamSettings(
    platform: PlatformInfo | null,
    encoders: EncoderInfo | null,
    caps: Capability[] | null,
    decoders: Decoder[] | null
) {
    const [s, setS] = useState<Stream | null>(null);
    const [preset, setPreset] = useState(CUSTOM_PRESET);
    const [userPresets, setUserPresets] = useState<Preset[]>([]);
    const [transports, setTransports] = useState<string[]>(["srt"]);
    const [carriage, setCarriage] = useState<TransportCarriage[] | null>(null);
    const [captureTransports, setCaptureTransports] =
        useState<Record<string, string[]> | null>(null);
    const [captureEngines, setCaptureEngines] =
        useState<Record<string, string> | null>(null);
    const [cmd, setCmd] = useState("");
    // Why each persisted store could not be read, empty when it was. The settings
    // notice is fixed at startup and the preset one follows the last preset action,
    // so they are held apart and joined for display instead of overwriting each
    // other. An unusable store file has been moved aside by the backend rather than
    // left for the next write to replace, so these sentences are what tell the user
    // the values still exist and where.
    const [settingsNotice, setSettingsNotice] = useState("");
    const [presetError, setPresetError] = useState("");

    // One value for every fact the dependency rules read, so the evaluation and
    // the repairs cannot be handed different subsets.
    const env: Environment = useMemo(
        () => ({
            platform, encoders, caps, decoders, carriage, captureTransports,
            captureEngines,
        }),
        [
            platform, encoders, caps, decoders, carriage, captureTransports,
            captureEngines,
        ]
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
        () => (s ? nativeGridCheck(s, caps, carriage) : null),
        [s, caps, carriage]
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

    // Every preset binding rejects when the presets file could not be read, and the
    // backend has moved that file aside by then. Reporting the reason is what tells
    // the empty list apart from a list that is empty because nothing was saved.
    const loadPresets = useCallback(async (): Promise<Preset[]> => {
        try {
            const presets = await GetPresets();
            setPresetError("");
            return presets;
        } catch (e) {
            setPresetError(String(e));
            return [];
        }
    }, []);

    const saveAsPreset = useCallback(
        async (name: string) => {
            const trimmed = name.trim();
            if (!s || !trimmed) {
                return;
            }
            try {
                await SavePreset(trimmed, s);
            } catch (e) {
                setPresetError(String(e));
                return;
            }
            setUserPresets(await loadPresets());
            setPreset(userPresetValue(trimmed));
        },
        [s, loadPresets]
    );

    const deletePreset = useCallback(
        async (value: string) => {
            if (!value.startsWith(USER_PREFIX)) {
                return;
            }
            try {
                await DeletePreset(value.slice(USER_PREFIX.length));
            } catch (e) {
                setPresetError(String(e));
                return;
            }
            setUserPresets(await loadPresets());
            setPreset(prev => (prev === value ? CUSTOM_PRESET : prev));
        },
        [loadPresets]
    );

    useEffect(() => {
        void (async () => {
            const loaded = normalize(await GetSettings());
            const presets = await loadPresets();
            setUserPresets(presets);
            setS(loaded);
            setPreset(matchPreset(loaded, presets));
            setSettingsNotice(await StoreNotice());
            setTransports(await Transports());
            setCarriage(await TransportFormats());
            setCaptureTransports(await CaptureTransports());
            setCaptureEngines(await CaptureEngines());
        })();
    }, [loadPresets]);

    // Re-normalize whenever a dimension resolves after mount: platform gates the
    // capture backend (ddagrab on Linux falls back), the encoder probe and capability
    // table gate the codec/chroma (hevc_nvenc drops to x264 without an NVIDIA
    // encoder), the transport table gates the codec (MPEG-TS carries no VP9), the
    // capture->transport map gates the transport (the GStreamer engine has no RTMP
    // sink), and the capture->engine map gates the chroma (the portal path's
    // encoders drop planar RGB). Any illegal carryover from the persisted settings
    // is repaired to a valid combination.
    useEffect(() => {
        if (platform || encoders || caps || carriage || captureTransports || captureEngines) {
            setS(prev => (prev ? normalize(prev, env) : prev));
        }
    }, [platform, encoders, caps, carriage, captureTransports, captureEngines, env]);

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
        storeError: [settingsNotice, presetError].filter(Boolean).join("\n"),
        update, applyPreset, saveAsPreset, deletePreset,
    };
}
