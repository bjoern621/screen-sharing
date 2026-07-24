import { useCallback, useEffect, useMemo, useState } from "react";
import {
    GetSettings, SaveSettings, GetPresets, SavePreset, DeletePreset,
    PublishCommand, Transports, CaptureTransports,
} from "../../wailsjs/go/main/App";
import {
    BrowserVerdict, Deps, EncoderInfo, PlatformInfo, Preset, Stream,
} from "../types/stream";
import { evaluateDeps, normalize } from "../util/deps";
import { Capability } from "../util/domain";
import { browserCheck } from "../util/browser";
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
 * dependency map, browser verdict, live command preview and the transport
 * list. Any field change re-normalizes the settings and drops the preset back to
 * "custom"; applying a preset patches many fields at once without doing so.
 * The platform gates which capture APIs are available, the encoder set which
 * codecs the machine can run, and the capability table which codec/chroma/
 * transport combinations are legal.
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
    const [cmd, setCmd] = useState("");

    const deps: Deps | null = useMemo(
        () => (s ? evaluateDeps(s, platform, encoders, caps, captureTransports) : null),
        [s, platform, encoders, caps, captureTransports]
    );
    const browser: BrowserVerdict | null = useMemo(
        () => (s ? browserCheck(s, caps) : null),
        [s, caps]
    );

    const update = useCallback(
        (patch: Partial<Stream>, fromPreset = false) => {
            setS(prev =>
                prev
                    ? normalize({ ...prev, ...patch } as Stream, platform, encoders, caps, captureTransports)
                    : prev
            );
            if (!fromPreset) {
                setPreset(CUSTOM_PRESET);
            }
        },
        [platform, encoders, caps, captureTransports]
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
        })();
    }, []);

    // Re-normalize whenever a dimension resolves after mount: platform gates the
    // capture API (ddagrab on Linux falls back), the encoder probe and capability
    // table gate the codec/chroma (hevc_nvenc drops to x264 without an NVIDIA
    // encoder), and the capture->transport map gates the transport (the portal
    // path drops WebRTC). Any illegal carryover from the persisted settings is
    // repaired to a valid combination.
    useEffect(() => {
        if (platform || encoders || caps || captureTransports) {
            setS(prev =>
                prev ? normalize(prev, platform, encoders, caps, captureTransports) : prev
            );
        }
    }, [platform, encoders, caps, captureTransports]);

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
        s, preset, userPresets, transports, deps, browser, cmd,
        update, applyPreset, saveAsPreset, deletePreset,
    };
}
