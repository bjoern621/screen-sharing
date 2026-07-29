import { useCallback, useEffect, useRef, useState } from "react";
import {
    StartPublish, StopPublish, GetPublishState, Republish,
} from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { PublishExit, PublishState, Stats, Stream } from "../types/stream";

const ROLLING_WINDOW_MS = 5000;

/** How long the figures may go without a fresh sample before they stop
 * describing the stream. Both engines report about once a second, so three
 * seconds is several missed reports, not a late one. */
const STALE_AFTER_SEC = 3;

/** Cadence of the ageing tick. A sample is what dates the figures, and a stalled
 * encoder sends none, so the window and the sample age are moved by the clock
 * rather than by arrival. */
const TICK_MS = 500;

/**
 * Manages the publish lifecycle and the derived insight figures: latest encoder
 * stats, the 5-second rolling mean bitrate, the session peak and how old the
 * newest sample is. The mean and the age come from a timer, so a stream that
 * stops reporting ages out instead of holding its last healthy figures.
 *
 * A null mean, peak or sample age is the absence of a measurement, matching the
 * convention the samples themselves carry. Reads the current settings so
 * toggling publish always sends the live values.
 *
 * `live` is the settings the running pipeline was built from and `pending` says they
 * are no longer the ones the form holds. Both come from the backend, which compares
 * the pipelines the two build: a running publish is a child process built from an
 * argv, so `apply` reaches the new values by relaunching it.
 */
export function usePublish(s: Stream | null) {
    const [publishing, setPublishing] = useState(false);
    const [live, setLive] = useState<Stream | null>(null);
    const [pending, setPending] = useState(false);
    const [error, setError] = useState("");
    const [logPath, setLogPath] = useState("");
    const [stats, setStats] = useState<Stats | null>(null);
    const [avg5, setAvg5] = useState<number | null>(null);
    const [peak, setPeak] = useState<number | null>(null);
    const [sampleAgeSec, setSampleAgeSec] = useState<number | null>(null);
    const history = useRef<{ t: number; mbps: number }[]>([]);
    const lastSampleAt = useRef<number | null>(null);

    /** Drops everything the window and the age are derived from, so a stopped
     * session shows dashes instead of the last values it happened to report. */
    const resetInsights = useCallback(() => {
        history.current = [];
        lastSampleAt.current = null;
        setStats(null);
        setAvg5(null);
        setPeak(null);
        setSampleAgeSec(null);
    }, []);

    /** Ages the rolling window out to now and restates the mean and the sample
     * age. Called on every sample and on every tick, since the figures go stale
     * exactly when no sample arrives. */
    const age = useCallback((now: number) => {
        const h = history.current;
        while (h.length && now - h[0].t > ROLLING_WINDOW_MS) {
            h.shift();
        }
        setAvg5(h.length ? h.reduce((a, b) => a + b.mbps, 0) / h.length : null);
        // Floored, so the age is one the newest sample has actually reached.
        setSampleAgeSec(
            lastSampleAt.current === null
                ? null
                : Math.floor((now - lastSampleAt.current) / 1000)
        );
    }, []);

    /** Takes the publish state the backend announced. The insights describe a running
     * pipeline, so they go with it: the figures a stopped stream reached last are not
     * figures of anything. */
    const take = useCallback(
        (state: PublishState) => {
            setPublishing(state.publishing);
            setLive(state.settings ?? null);
            setPending(state.pending);
            if (!state.publishing) {
                resetInsights();
            }
        },
        [resetInsights]
    );

    useEffect(() => {
        // Read once at mount, then followed by event: a window that opens over a
        // publish somebody else started shows the same state as one that watched it
        // start.
        void (async () => take(await GetPublishState()))();

        const offStats = EventsOn("publish:stats", (st: Stats) => {
            const now = Date.now();
            lastSampleAt.current = now;
            const inst = st.instMbps;
            // A measured zero belongs in the window: it is throughput the mean
            // has to carry. Only an unmeasured sample stays out of it.
            if (inst !== null) {
                history.current.push({ t: now, mbps: inst });
                setPeak(prev => (prev === null ? inst : Math.max(prev, inst)));
            }
            age(now);
            setStats(st);
        });

        // The publish state also moves without this window: the native grid's sidebar
        // reaches the same publish, and every settings write moves whether the running
        // pipeline still carries what the form shows.
        const offState = EventsOn("publish:state", take);

        // The exit carries why a pipeline ended and where its log is. That it ended is
        // the state event's to report, so a stream that was stopped on purpose does not
        // arrive here as an event with nothing to say.
        const offExit = EventsOn("publish:exit", (e: PublishExit) => {
            setLogPath(e.logPath ?? "");
            if (e.message) {
                setError("Publisher exited: " + e.message);
            }
        });

        return () => {
            offStats();
            offState();
            offExit();
        };
    }, [age, take]);

    useEffect(() => {
        if (!publishing) {
            return;
        }
        const id = setInterval(() => age(Date.now()), TICK_MS);
        return () => clearInterval(id);
    }, [publishing, age]);

    // The state the call moves is announced by the backend, so nothing is written here
    // on the way: one owner reports what publishing became, and a start that failed
    // leaves the button on the state the app is actually in rather than on the one this
    // window asked for.
    const toggle = useCallback(async () => {
        setError("");
        setLogPath("");
        try {
            if (publishing) {
                await StopPublish();
            } else if (s) {
                resetInsights();
                await StartPublish(s);
            }
        } catch (e) {
            setError("Publishing failed: " + e);
        }
    }, [publishing, s, resetInsights]);

    // Relaunches the live pipeline on the settings the form holds. The figures belong
    // to the pipeline they were measured on, so they start over rather than carrying a
    // peak the new encoder never reached.
    const apply = useCallback(async () => {
        if (!s) {
            return;
        }
        setError("");
        setLogPath("");
        resetInsights();
        try {
            // The settings travel with the call rather than being read off the backend,
            // which holds them on a debounce: a click inside that window would restart
            // the stream onto the edit before this one.
            await Republish(s);
        } catch (e) {
            setError("Applying the settings to the live stream failed: " + e);
        }
    }, [s, resetInsights]);

    const stale = sampleAgeSec !== null && sampleAgeSec >= STALE_AFTER_SEC;

    return {
        publishing, live, pending, error, logPath, stats, avg5, peak, sampleAgeSec,
        stale, toggle, apply,
    };
}
