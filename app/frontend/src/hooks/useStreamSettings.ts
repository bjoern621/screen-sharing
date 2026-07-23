import { useCallback, useEffect, useMemo, useState } from "react";
import {
    GetSettings, SaveSettings, PublishCommand, Transports,
} from "../../wailsjs/go/main/App";
import { BrowserVerdict, Deps, PlatformInfo, Stream } from "../types/stream";
import { evaluateDeps, normalize } from "../util/deps";
import { browserCheck } from "../util/browser";
import { PRESETS } from "../util/presets";

const CUSTOM_PRESET = "custom";

/**
 * Owns the editable stream settings and everything derived from them: the
 * dependency map, browser verdict, live ffmpeg command preview and the transport
 * list. Any field change re-normalizes the settings and drops the preset back to
 * "custom"; applying a preset patches many fields at once without doing so.
 * The platform gates which capture APIs are available.
 */
export function useStreamSettings(platform: PlatformInfo | null) {
    const [s, setS] = useState<Stream | null>(null);
    const [preset, setPreset] = useState(CUSTOM_PRESET);
    const [transports, setTransports] = useState<string[]>(["srt"]);
    const [cmd, setCmd] = useState("");

    const deps: Deps | null = useMemo(
        () => (s ? evaluateDeps(s, platform) : null),
        [s, platform]
    );
    const browser: BrowserVerdict | null = useMemo(
        () => (s ? browserCheck(s) : null),
        [s]
    );

    const update = useCallback(
        (patch: Partial<Stream>, fromPreset = false) => {
            setS(prev =>
                prev ? normalize({ ...prev, ...patch } as Stream, platform) : prev
            );
            if (!fromPreset) {
                setPreset(CUSTOM_PRESET);
            }
        },
        [platform]
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
            setS(normalize(await GetSettings()));
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
