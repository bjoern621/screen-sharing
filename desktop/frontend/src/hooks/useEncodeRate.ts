import { useCallback, useState } from "react";
import { MeasureEncodeRate } from "../../wailsjs/go/app/App";
import { EncodeRate, Stream } from "../types/stream";

/**
 * The settings an encode measurement describes, as one string to compare against.
 *
 * Every field here changes what a frame costs the encoder: which engine runs it
 * (`capture`), which encoder and how it is driven, how large the picture is
 * (`monitor`), and where the frames reach it from.
 *
 * `fps` is deliberately absent. It is the figure the measurement is judged against,
 * and invalidating the measurement whenever it moves would take the answer away on
 * the one knob a user reaches for after reading it. What it does reach into is the
 * automatic keyframe interval, so a target change moves the measured rate by roughly
 * what one keyframe more or less per two seconds costs, which is well inside the
 * content range the figure already spans.
 */
function fingerprint(s: Stream): string {
    return [
        s.capture,
        s.captureMemory,
        s.monitor,
        s.codec,
        s.mode,
        s.chroma,
        s.colorRange,
        s.cq,
        s.bitrateM,
        s.maxrateM,
        s.vbvMs,
        s.gop,
        s.bframes,
        s.encPreset,
    ].join("|");
}

/**
 * Owns the measured encode rate for the current settings: the figure, the request
 * that produces it, and whether it still describes what the form shows.
 *
 * The figure is not persisted, unlike the uplink one beside it. A line's capacity is
 * a property of the line and survives a restart; an encode rate is a property of
 * these settings on this machine, and a stored one would come back after a codec
 * change describing an encoder that is no longer selected. So it lives for as long as
 * the settings it was taken under, and says so the moment they move.
 */
export function useEncodeRate(s: Stream | null) {
    const [rate, setRate] = useState<EncodeRate | null>(null);
    const [measuredFor, setMeasuredFor] = useState("");
    const [measuring, setMeasuring] = useState(false);
    const [error, setError] = useState("");

    const measure = useCallback(async () => {
        if (!s) {
            return;
        }
        setError("");
        setMeasuring(true);
        try {
            const value = await MeasureEncodeRate(s);
            setRate(value);
            setMeasuredFor(fingerprint(s));
        } catch (e) {
            setRate(null);
            setMeasuredFor("");
            setError("Measuring the encode rate failed: " + e);
        } finally {
            setMeasuring(false);
        }
    }, [s]);

    // Read through on every render rather than tracked as state of its own: the
    // settings are the source of truth for whether the figure still applies, and a
    // stored flag would be one that goes stale exactly when the settings do.
    const stale = rate !== null && !!s && fingerprint(s) !== measuredFor;

    return { rate, stale, measuring, error, measure };
}
