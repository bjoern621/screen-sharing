import { Preset, Stream } from "../types/stream";
import { holds } from "./claim";
import { Environment, applyIntact, autoCaptures } from "./deps";
import {
    Capability, FAMILY_META, FORMAT_META, Family, Format, scaleCq,
} from "./domain";
import { CUSTOM_PRESET, PRESETS, Rung, userPresetValue } from "./presets";

/**
 * Applying a preset and reading one back off the settings.
 *
 * Both are searches over the same tables the rest of the form derives from, which
 * is what keeps a preset from ever offering a configuration the settings rules
 * would repair on arrival. See `docs/presets.md`.
 */

/**
 * Codecs a preset tries at one rung, best first: the encoders that come with a
 * device before the ones that come with a build, and coding efficiency inside each
 * half, in opposite directions.
 *
 * An encoder on fixed-function silicon leaves the machine free to run whatever is
 * being captured, which is what every preset here is for, so one is taken wherever
 * it reaches the rung.
 *
 * Efficiency then orders each half, because a format spends fewer bits by searching
 * more for them. On dedicated silicon that search costs nothing, so the most
 * efficient format wins; on a CPU it is the frame rate, so the cheapest one wins and
 * the ladder does not hand a desktop encode to the slowest encoder in the table.
 * Codecs that tie keep the capability table's order.
 *
 * An unresolved capability table names nothing, and the settings' own codec is what
 * the search then offers: an unresolved fact restricts nothing, as everywhere in the
 * dependency rules.
 */
function codecOrder(
    codec: string,
    rung: Rung,
    caps: Capability[] | null
): string[] {
    if (!caps) {
        return [codec];
    }
    const onDevice = (c: Capability) =>
        !!FAMILY_META[c.family as Family]?.needsDevice;
    const bits = (c: Capability) =>
        FORMAT_META[c.format as Format]?.efficiency ?? 1;
    const rank = (c: Capability) => (onDevice(c) ? bits(c) : -bits(c));
    return caps
        .filter(c => c.implemented && (!rung.onDevice || onDevice(c)))
        .sort(
            (a, b) =>
                Number(onDevice(b)) - Number(onDevice(a)) || rank(a) - rank(b)
        )
        .map(c => c.name);
}

/**
 * Capture backends a preset tries, the selected one first. A configuration reachable
 * without changing the backend is therefore the one taken, and the compositor's
 * picker is not raised for a preset that had no need of it.
 */
function captureOrder(capture: string, env: Environment): string[] {
    return [capture, ...autoCaptures(env.platform).filter(c => c !== capture)];
}

/**
 * The settings applying this preset produces here, or null where nothing this
 * machine runs delivers its claim.
 *
 * The ladder is walked rung by rung, each rung against every codec and each codec
 * against every capture backend, and the first candidate that survives
 * normalization intact is the answer. Rung above codec above capture backend is
 * what makes the ladder the preset's statement of what it gives up: the search
 * changes encoder, and then capture backend, to stay on a rung it can still reach.
 *
 * A candidate is taken only when it also delivers the claim, so a base that
 * contradicts the preset's own promise leaves the preset unavailable rather than
 * applying something the selector would then drop straight back out of.
 *
 * Applying twice equals applying once: the settings this returns are themselves the
 * candidate the next search finds first, since the rung, codec and capture backend
 * that produced them are the ones it reaches first.
 */
export function resolvePreset(
    key: string,
    s: Stream,
    env: Environment
): Stream | null {
    const spec = PRESETS[key];
    if (!spec) {
        return null;
    }
    for (const rung of spec.rungs) {
        for (const codec of codecOrder(s.codec, rung, env.caps)) {
            const cq =
                spec.cq51 === undefined
                    ? {}
                    : { cq: scaleCq(spec.cq51, codec, env.caps) };
            for (const capture of captureOrder(s.capture, env)) {
                const next = applyIntact(
                    s,
                    { ...spec.base, ...cq, chroma: rung.chroma, codec, capture },
                    env
                );
                if (next && holds(next, spec.claim)) {
                    return next;
                }
            }
        }
    }
    return null;
}

/**
 * The presets no configuration reaches on this machine, each mapped to the reason.
 *
 * The reason names the publish transport, which is the one dimension the search
 * leaves alone: the transport is how viewers are reached rather than a property of
 * the picture, so a preset never moves it, and a transport whose bitstream formats
 * rule out every candidate is a thing the user can act on.
 */
export function unreachablePresets(
    s: Stream,
    env: Environment
): Record<string, string> {
    const out: Record<string, string> = {};
    for (const [key, spec] of Object.entries(PRESETS)) {
        if (!resolvePreset(key, s, env)) {
            out[key] =
                `nothing this machine runs reaches ${spec.needs} over the ${s.transport} publish transport`;
        }
    }
    return out;
}

/** True field-by-field equality of two settings snapshots. */
function streamEquals(a: Stream, b: Stream): boolean {
    const keys = Object.keys(a) as (keyof Stream)[];
    return keys.every(k => a[k] === b[k]);
}

/**
 * The selector value the given settings correspond to: a saved preset they equal
 * field for field, else the built-in preset whose claim they deliver, else custom.
 *
 * This is the whole of what the selector shows. The selection is read from the
 * settings on every change rather than remembered from the click that applied it,
 * so a field edited to a value the preset still covers keeps the preset, and one
 * edited past its claim drops out of it, with no stored answer to disagree.
 *
 * A saved preset wins over a claim because it is the more exact statement: it says
 * every field, where a claim says which settings deliver a promise.
 */
export function matchPreset(s: Stream, saved: Preset[]): string {
    const user = saved.find(p => streamEquals(s, p.settings));
    if (user) {
        return userPresetValue(user.name);
    }
    const built = Object.entries(PRESETS).find(([, p]) => holds(s, p.claim));
    return built ? built[0] : CUSTOM_PRESET;
}
