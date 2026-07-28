import { Option, Stream } from "../types/stream";
import { Claim, overlaps } from "./claim";
import { Chroma } from "./domain";

/**
 * The built-in presets. A preset is a claim plus a ladder of configurations that
 * reach it, not a snapshot of settings: which encoder, pixel format and capture
 * backend deliver the claim is the machine's answer and differs per machine, so this
 * table states the goal and `presetSearch.ts` finds a way to it.
 *
 * See `docs/presets.md` for the model and the rules the table is written to.
 */

/** Selector value for settings that meet no preset's claim. */
export const CUSTOM_PRESET = "custom";

/** User presets share the dropdown with the built-in ones, so their selector values
 * are namespaced to never collide with a built-in key or "custom". The namespacing
 * is this module's business: everywhere else goes through the three calls below. */
const USER_PREFIX = "user:";

/** Selector value for the saved preset of this name. */
export function userPresetValue(name: string): string {
    return USER_PREFIX + name;
}

/** Whether a selector value names one of the user's saved presets. */
export function isUserPreset(value: string): boolean {
    return value.startsWith(USER_PREFIX);
}

/** The saved preset a selector value names, empty for a built-in one. */
export function savedPresetName(value: string): string {
    return isUserPreset(value) ? value.slice(USER_PREFIX.length) : "";
}

/**
 * One step of a preset's quality ladder: the picture it asks for, and whether an
 * encoder that comes with a device is the only one allowed to serve it.
 *
 * The device restriction is what lets a ladder put a lesser picture above a better
 * one on the CPU. Where a preset's own trade does not need that, the rung takes
 * whichever encoder reaches it.
 */
export interface Rung {
    chroma: Chroma;
    onDevice?: boolean;
}

export interface PresetSpec {
    label: string;
    /**
     * What the preset delivers, in one line. It is true of every settings object
     * the claim accepts and of no other, which is what lets the claim alone decide
     * whether the preset is still the selected one after a field changed.
     */
    hint: string;
    /** What every candidate needs, as a noun phrase, for the sentence shown where
     * no candidate is reachable. */
    needs: string;
    /** The region of the settings space this preset stands for. */
    claim: Claim;
    /** Fields every candidate sets. The rate-control recipe is the preset's
     * identity, so it is fixed here rather than searched for. */
    base: Partial<Stream>;
    /** Quantizer target on the 51-point scale, rescaled to each candidate codec's
     * own. Absent where the preset's mode targets no quantizer. */
    cq51?: number;
    /** The quality ladder, best first. The search takes the highest rung an encoder
     * here reaches, so the order is where the preset states what it gives up first. */
    rungs: Rung[];
}

/**
 * Every claim is disjoint from every other (see the guard below), so at most one of
 * these ever describes one settings object.
 */
export const PRESETS: Record<string, PresetSpec> = {
    lossless: {
        label: "Lossless",
        hint: "Bit-exact pixels: the encoder quantizes nothing and no chroma is thrown away. Bursts to hundreds of Mbit/s on motion, so LAN and localhost only.",
        needs: "lossless coding at full-resolution chroma",
        claim: { modes: ["lossless"], chromas: ["gbrp", "yuv444p"] },
        base: {
            mode: "lossless", colorRange: "pc", fps: 60, gop: 0, bframes: 0,
            encPreset: "p7",
            // Loss on a LAN is near zero, so the two windows only have to absorb
            // scheduling jitter rather than a WAN's retransmits.
            srtPublishLatencyMs: 60, srtWatchLatencyMs: 120,
        },
        // Planar RGB is the desktop's own format and reaches the encoder without a
        // color conversion; 4:4:4 carries the same detail after one.
        //
        // The two CPU rungs run the other way round. A software encoder codes
        // lossless 4:4:4 an order of magnitude faster than it codes lossless RGB, and
        // an encode that cannot keep up with the screen delivers neither format, so
        // the exact one is what this ladder gives up last rather than first.
        rungs: [
            { chroma: "gbrp", onDevice: true },
            { chroma: "yuv444p", onDevice: true },
            { chroma: "yuv444p" },
            { chroma: "gbrp" },
        ],
    },
    gaming: {
        label: "Gaming",
        hint: "Motion first: 60 fps or more, no B-frame reorder delay, short retransmit windows, and a bitrate held constant so a busy scene costs no extra delay.",
        needs: "a constant bitrate at 60 fps with no reorder delay",
        claim: {
            modes: ["cbr", "vbr", "abr", "crf"],
            fps: { min: 60 },
            bframes: { max: 0 },
            srtTotalMs: { max: 500 },
        },
        base: {
            mode: "cbr", colorRange: "pc", fps: 60, gop: 0, bframes: 0,
            encPreset: "p5", bitrateM: 40,
            // Around six frames of rate buffer at 60 fps: room for the encoder to
            // carry the target across a scene change, short enough that the buffer
            // itself adds no delay a player would show.
            vbvMs: 100,
            srtPublishLatencyMs: 100, srtWatchLatencyMs: 200,
        },
        // Quarter-resolution chroma is the cheapest encode and the one every
        // encoder here codes, which is what keeps the frame rate up on motion.
        rungs: [{ chroma: "yuv420p" }],
    },
    readability: {
        label: "High readability",
        hint: "Text first: constant quality at a screen-share frame rate, so a still page of text gets the bits that motion would otherwise take.",
        needs: "constant-quality coding at a screen-share frame rate",
        claim: { modes: ["crf"], fps: { max: 30 } },
        base: {
            mode: "crf", colorRange: "pc", fps: 30, gop: 0, bframes: 2,
            encPreset: "p7", bitrateM: 150,
            srtPublishLatencyMs: 300, srtWatchLatencyMs: 1200,
        },
        cq51: 18,
        // Full-resolution chroma keeps the edges of colored glyphs where they are,
        // and 30 fps of it is within reach of a CPU encoder, so this rung takes
        // whichever encoder codes it. Quarter-resolution chroma still carries
        // full-resolution luma, which is most of what makes text legible, so it is
        // the rung below rather than a reason to be unavailable.
        rungs: [{ chroma: "yuv444p" }, { chroma: "yuv420p" }],
    },
};

// Two presets whose claims intersect would both describe one settings object, and
// the selector has one value to show. The claims are written to part on an axis:
// the rate-control mode tells lossless from the other two, and the frame rate tells
// those two apart. This holds the table to it, so a claim widened past its
// neighbour fails here rather than at a selector left to pick one of two answers.
for (const [a, x] of Object.entries(PRESETS)) {
    for (const [b, y] of Object.entries(PRESETS)) {
        if (a < b && overlaps(x.claim, y.claim)) {
            throw new Error(
                `preset claims are pairwise disjoint: ${a} and ${b} both accept one settings object`
            );
        }
    }
}

/** Label and hint for the state of matching no preset. */
const CUSTOM_LABEL = "Custom";
const CUSTOM_HINT =
    "The settings match no preset. Changing a field keeps the preset selected as long as the setting still delivers what the preset promises.";

/** The built-in entries of the preset selector, custom first. */
export function presetOptions(): Option[] {
    return [
        { value: CUSTOM_PRESET, label: CUSTOM_LABEL, tip: CUSTOM_HINT },
        ...Object.entries(PRESETS).map(([value, p]) => ({
            value,
            label: p.label,
            tip: p.hint,
        })),
    ];
}

/** The sentence shown beside the selector for the current selection. */
export function presetHint(value: string): string {
    if (isUserPreset(value)) {
        return "A saved snapshot of every field, repaired only where this machine cannot run a value it holds.";
    }
    return PRESETS[value]?.hint ?? CUSTOM_HINT;
}
