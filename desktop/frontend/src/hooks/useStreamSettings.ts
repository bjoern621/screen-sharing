import { useCallback, useEffect, useMemo, useState } from "react";
import {
    GetSettings, SaveSettings, GetPresets, SavePreset, DeletePreset,
    PublishCommand, AudioCodecs, TransportFormats, CaptureTransports,
    CaptureEngines, GpuPaths, StoreNotice,
} from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import {
    Deps, EncoderInfo, GpuPath, PlatformInfo, Preset, Stream, TransportCarriage,
    ViewVerdict,
} from "../types/stream";
import {
    Environment, evaluateDeps, normalize, publishTransports,
} from "../util/deps";
import {
    AudioCodec, Capability, Decoder, Engine, engineFor,
} from "../util/domain";
import { nativeGridCheck } from "../util/nativegrid";
import { webGridCheck } from "../util/webgrid";
import {
    CUSTOM_PRESET, isUserPreset, savedPresetName, userPresetValue,
} from "../util/presets";
import {
    matchPreset, resolvePreset, unreachablePresets,
} from "../util/presetSearch";

/**
 * Owns the editable stream settings and everything derived from them: the
 * dependency map, the selected preset, the web- and native-grid verdicts, live
 * command preview and the transport list.
 *
 * The selected preset is derived from the settings rather than remembered from the
 * click that applied it, so a field edited to a value the preset still covers keeps
 * the preset and one edited past its claim leaves it. Applying a preset searches for
 * a configuration this machine can run and writes the whole result at once; a preset
 * no configuration reaches carries the reason instead.
 *
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
    const [userPresets, setUserPresets] = useState<Preset[]>([]);
    const [audioCodecs, setAudioCodecs] = useState<AudioCodec[] | null>(null);
    const [carriage, setCarriage] = useState<TransportCarriage[] | null>(null);
    const [captureTransports, setCaptureTransports] =
        useState<Record<string, string[]> | null>(null);
    const [captureEngines, setCaptureEngines] =
        useState<Record<string, string> | null>(null);
    const [gpuPaths, setGpuPaths] = useState<GpuPath[] | null>(null);
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
            platform, encoders, caps, decoders, audioCodecs, carriage,
            captureTransports, captureEngines, gpuPaths,
        }),
        [
            platform, encoders, caps, decoders, audioCodecs, carriage,
            captureTransports, captureEngines, gpuPaths,
        ]
    );

    // The publish-transport roster: what the capture-to-transport map spans, which
    // is every transport one of the engines carries. There is no engine-neutral
    // roster to fetch, since a union over both engines is the narrowing the
    // per-engine tables exist to remove; the entries this capture's engine has no
    // sink for are greyed by evaluateDeps from the same map.
    const transports = useMemo(
        () => publishTransports(captureTransports, s?.transport ?? ""),
        [captureTransports, s]
    );

    const deps: Deps | null = useMemo(
        () => (s ? evaluateDeps(s, env) : null),
        [s, env]
    );
    const preset = useMemo(
        () => (s ? matchPreset(s, userPresets) : CUSTOM_PRESET),
        [s, userPresets]
    );
    // Presets no configuration on this machine reaches, so the selector greys each
    // with the reason instead of applying a preset that would be repaired into
    // something else on arrival.
    const presetDisabled = useMemo(
        () => (s ? unreachablePresets(s, env) : {}),
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
        (patch: Partial<Stream>) => {
            setS(prev =>
                prev ? normalize({ ...prev, ...patch } as Stream, env) : prev
            );
        },
        [env]
    );

    // A saved preset is a snapshot, applied whole and repaired only where this
    // machine cannot run a value it holds: it was written on some machine and may
    // name a codec or capture backend this one lacks. A built-in preset is a claim,
    // and the search answers which configuration delivers it here, so a repair would
    // be a different configuration under the preset's name. One with no reachable
    // configuration is greyed in the selector, and this leaves the settings alone.
    const applyPreset = useCallback(
        (value: string) => {
            if (!s) {
                return;
            }
            if (isUserPreset(value)) {
                const saved = userPresets.find(
                    p => userPresetValue(p.name) === value
                );
                if (saved) {
                    setS(normalize(saved.settings, env));
                }
                return;
            }
            const next = resolvePreset(value, s, env);
            if (next) {
                setS(next);
            }
        },
        [s, env, userPresets]
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
        },
        [s, loadPresets]
    );

    const deletePreset = useCallback(
        async (value: string) => {
            const name = savedPresetName(value);
            if (!name) {
                return;
            }
            try {
                await DeletePreset(name);
            } catch (e) {
                setPresetError(String(e));
                return;
            }
            setUserPresets(await loadPresets());
        },
        [loadPresets]
    );

    useEffect(() => {
        void (async () => {
            setUserPresets(await loadPresets());
            setS(normalize(await GetSettings()));
            setSettingsNotice(await StoreNotice());
            setAudioCodecs(await AudioCodecs());
            setCarriage(await TransportFormats());
            setCaptureTransports(await CaptureTransports());
            setCaptureEngines(await CaptureEngines());
            setGpuPaths(await GpuPaths());
        })();
    }, [loadPresets]);

    // Re-normalize whenever a dimension resolves after mount: platform gates the
    // capture backend (ddagrab on Linux falls back), the encoder probe and capability
    // table gate the codec/chroma (hevc_nvenc drops to x264 without an NVIDIA
    // encoder), the transport table gates the codec (MPEG-TS carries no VP9), the
    // audio table gates the audio codec (WebRTC negotiates no AAC), the
    // capture->transport map gates the transport (the GStreamer engine has no RTMP
    // sink), the capture->engine map gates the chroma (the portal path's encoders drop
    // planar RGB), and the GPU pair table gates the frame memory (a direct path exists
    // for some capture and codec pairs and not others). Any illegal carryover from the
    // persisted settings is repaired to a valid combination.
    useEffect(() => {
        if (platform || encoders || caps || audioCodecs || carriage || captureTransports || captureEngines || gpuPaths) {
            setS(prev => (prev ? normalize(prev, env) : prev));
        }
    }, [platform, encoders, caps, audioCodecs, carriage, captureTransports, captureEngines, gpuPaths, env]);

    // The native grid's sidebar writes the watch leg and knobs it was moved to
    // into these same settings, and the backend announces it. Taking the change
    // is what keeps one copy: this hook writes the whole struct back on the next
    // field edit, which would otherwise put the leg the grid left back in force.
    useEffect(() => {
        const off = EventsOn("settings:changed", () => {
            void (async () => setS(normalize(await GetSettings(), env)))();
        });
        return () => off();
    }, [env]);

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
        s, preset, presetDisabled, userPresets, transports, audioCodecs, engine,
        deps, webGrid, nativeGrid, cmd,
        storeError: [settingsNotice, presetError].filter(Boolean).join("\n"),
        update, applyPreset, saveAsPreset, deletePreset,
    };
}
