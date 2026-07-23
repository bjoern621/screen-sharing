import { useCallback, useEffect, useMemo, useState } from "react";
import {
    GetSettings, SaveSettings, PublishCommand, Transports,
} from "../../wailsjs/go/main/App";
import { BrowserVerdict, Deps, EncoderInfo, PlatformInfo, Stream } from "../types/stream";
import { evaluateDeps, normalize } from "../util/deps";
import { Capability } from "../util/domain";
import { browserCheck } from "../util/browser";
import { PRESETS } from "../util/presets";

const CUSTOM_PRESET = "custom";

// Preset selected and applied on first launch, before the user touches anything.
const DEFAULT_PRESET = "lossless-rgb";

/**
 * Owns the editable stream settings and everything derived from them: the
 * dependency map, browser verdict, live ffmpeg command preview and the transport
 * list. Any field change re-normalizes the settings and drops the preset back to
 * "custom"; applying a preset patches many fields at once without doing so.
 * The platform gates which capture APIs are available, the encoder set which
 * codecs the machine can run, and the capability table which codec/chroma/
 * transport combinations are legal.
 */
export function useStreamSettings(
    platform: PlatformInfo | null,
    encoders: EncoderInfo | null,
    caps: Capability[] | null
) {
    const [s, setS] = useState<Stream | null>(null);
    const [preset, setPreset] = useState(DEFAULT_PRESET);
    const [transports, setTransports] = useState<string[]>(["srt"]);
    const [cmd, setCmd] = useState("");

    const deps: Deps | null = useMemo(
        () => (s ? evaluateDeps(s, platform, encoders) : null),
        [s, platform, encoders]
    );
    const browser: BrowserVerdict | null = useMemo(
        () => (s ? browserCheck(s) : null),
        [s]
    );

    const update = useCallback(
        (patch: Partial<Stream>, fromPreset = false) => {
            setS(prev =>
                prev ? normalize({ ...prev, ...patch } as Stream, platform, encoders) : prev
            );
            if (!fromPreset) {
                setPreset(CUSTOM_PRESET);
            }
        },
        [platform, encoders]
    );

    const applyPreset = useCallback(
        (name: string) => {
            setPreset(name);
            const p = PRESETS[name];
            if (p) {
                update(p, true);
            }
        },
        [update]
    );

    const save = useCallback(() => {
        if (s) {
            void SaveSettings(s);
        }
    }, [s]);

    useEffect(() => {
        void (async () => {
            const loaded = normalize(await GetSettings());
            const p = PRESETS[DEFAULT_PRESET];
            setS(p ? normalize({ ...loaded, ...p } as Stream) : loaded);
            setTransports(await Transports());
        })();
    }, []);

    // Once the platform is known, re-normalize so a default capture API that
    // this OS/session cannot run (e.g. ddagrab on Linux) falls back to a valid one.
    useEffect(() => {
        if (platform) {
            setS(prev => (prev ? normalize(prev, platform) : prev));
        }
    }, [platform]);

    // Once the encoder probe resolves, re-normalize so the default hevc_nvenc
    // codec drops to software x264 on a machine without a working NVIDIA encoder.
    useEffect(() => {
        if (encoders) {
            setS(prev => (prev ? normalize(prev, platform, encoders) : prev));
        }
    }, [encoders, platform]);

    useEffect(() => {
        if (!s) {
            return;
        }
        PublishCommand(s)
            .then(setCmd)
            .catch(e => setCmd(String(e)));
    }, [s]);

    return { s, preset, transports, deps, browser, cmd, update, applyPreset, save };
}
