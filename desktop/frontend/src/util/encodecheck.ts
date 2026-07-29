import { EncodeRate, Stream } from "../types/stream";

/**
 * Which side of the measured encode range the target frame rate falls on.
 *
 * - `over`: above everything the encoder reached, and the probe found that ceiling.
 *   The target is unreachable on any content, so frames are discarded from the first
 *   second.
 * - `unmeasured`: above everything the encoder reached, but the probe was the limit
 *   rather than the encoder, so where the ceiling actually sits is unknown.
 * - `content`: inside the range. Whether the encoder keeps up depends on what is on
 *   the screen.
 * - `ok`: below everything the encoder reached, so no content pushes it off the rate.
 */
export type EncodeStanding = "ok" | "content" | "unmeasured" | "over";

/** The measured rate and what it says about the configured target. */
export interface EncodeVerdict {
    standing: EncodeStanding;
    text: string;
}

/** Rounds a rate for display. Tenths below ten, whole numbers above, so a slow
 * combination keeps the digit that distinguishes it. */
function fps(value: number): string {
    return value >= 10 ? Math.round(value).toString() : value.toFixed(1);
}

/** The measured range as a figure. An end the probe could not out-run is written as
 * the floor it is, so a range with one open end does not read as two closed ones. */
export function formatEncodeRate(rate: EncodeRate): string {
    const low = rate.lowBounded
        ? `at least ${fps(rate.lowFps)}`
        : fps(rate.lowFps);
    const high = rate.highBounded
        ? `at least ${fps(rate.highFps)}`
        : fps(rate.highFps);
    return `${low}–${high} fps`;
}

/**
 * What the measured encode rate says about the configured target rate.
 *
 * The comparison is against both ends because the encoder's throughput depends on
 * content: a target under the low end is safe whatever is on screen, one over the
 * high end is unreachable whatever is on screen, and one between them is neither.
 *
 * A high end the frame generator paced is not a ceiling and does not refuse a target
 * above it. The probe stopped feeding the encoder before the encoder stopped keeping
 * up, so what it found is the floor of the easiest content, and calling a target
 * unreachable against it would refuse a rate that was never measured.
 *
 * This is the encoder's half of the same question the uplink field asks about the
 * line, and it is worth asking for the same reason. Neither failure announces itself:
 * an encoder that cannot keep up does not slow the stream down, it drops the frames
 * it cannot take, and what publishes is a lower rate than the one the form shows with
 * no error anywhere.
 */
export function encodeCheck(s: Stream, rate: EncodeRate): EncodeVerdict {
    const measured = formatEncodeRate(rate);

    if (s.fps > rate.highFps) {
        if (rate.highBounded) {
            // The low end is usually a real measurement even where the high one is
            // not, and it is the actionable half: content that codes anywhere near
            // the hard end misses the target whatever the ceiling turns out to be.
            const floor = rate.lowBounded
                ? ""
                : ` The hardest content it was measured on reached ${fps(rate.lowFps)} fps, so detailed or moving content misses the target whatever that ceiling is.`;
            return {
                standing: "unmeasured",
                text: `Target ${s.fps} fps is above the ${measured} measured here, but the probe could not generate frames faster than this encoder took them, so the high end is a floor rather than the encoder's ceiling and whether the target is reachable on easy content went unmeasured.${floor} The encoder column under Publish insights answers it for real content.`,
            };
        }
        return {
            standing: "over",
            text: `Target ${s.fps} fps is above the ${measured} this machine encoded these settings at, on the easiest content it was measured on. The encoder cannot reach the target on anything, so the frames it cannot take are discarded before it and the stream publishes at the rate it manages. Lower the target, or lower what a frame costs: a cheaper rate-control mode, a subsampled chroma, a faster encoder preset, or a hardware encoder.`,
        };
    }
    if (s.fps > rate.lowFps) {
        return {
            standing: "content",
            text: `Target ${s.fps} fps sits inside the ${measured} this machine encoded these settings at. A still desktop reaches it and detailed or moving content does not, and the frames the encoder cannot take on the way are discarded before it.`,
        };
    }
    return {
        standing: "ok",
        text: `Target ${s.fps} fps is below the ${measured} this machine encoded these settings at, including on the hardest content it was measured on, so no content pushes the encoder off the target.`,
    };
}
