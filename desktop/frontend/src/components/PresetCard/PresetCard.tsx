import { IconTrash } from "@tabler/icons-react";
import {
    Select, SelectContent, SelectGroup, SelectItem, SelectLabel,
    SelectSeparator, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Option, Preset } from "../../types/stream";
import {
    CUSTOM_PRESET, isUserPreset, presetHint, presetOptions, userPresetValue,
} from "../../util/presets";
import { labelFor } from "../../util/options";
import OptionRow from "../OptionRow/OptionRow";

interface PresetCardProps {
    preset: string;
    userPresets: Preset[];
    /** Built-in preset -> reason no configuration on this machine delivers it. */
    presetDisabled: Record<string, string>;
    /** Applying a preset mutates the stream settings, which is unsupported
     * mid-stream, so the selector is disabled while a stream is live. */
    publishing: boolean;
    onApplyPreset: (name: string) => void;
    onDeletePreset: (value: string) => void;
}

/** Master preset selector: built-in presets plus the user's saved ones, with a
 * one-line description of the current choice and a delete control for saved
 * presets. */
export default function PresetCard({
    preset,
    userPresets,
    presetDisabled,
    publishing,
    onApplyPreset,
    onDeletePreset,
}: PresetCardProps) {
    const options = presetOptions();
    const saved: Option[] = userPresets.map(p => ({
        value: userPresetValue(p.name),
        label: p.name,
    }));
    // Custom is a state and not a choice: the settings match no preset, and the way
    // out of it is to pick one. It carries that as its reason rather than being
    // hidden, so the selector can show it as the current value and say what it means.
    const reasonFor = (value: string) =>
        value === CUSTOM_PRESET
            ? "no preset to switch to - this is what the selector reads while the settings match none"
            : presetDisabled[value];

    return (
        <Card>
            <CardHeader>
                <CardTitle className="text-base">Preset</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-wrap items-center gap-3">
                <Select
                    value={preset}
                    disabled={publishing}
                    onValueChange={(v: string | null) => v && onApplyPreset(v)}
                >
                    <SelectTrigger className="w-72">
                        <SelectValue>
                            {(v: string) => labelFor([...options, ...saved], v)}
                        </SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                        <SelectGroup>
                            {options.map(o => (
                                <SelectItem
                                    key={o.value}
                                    value={o.value}
                                    disabled={!!reasonFor(o.value)}
                                    // Base UI sets pointer-events-none on disabled
                                    // items, which would swallow the hover that opens
                                    // the option's tooltip. Restore pointer events so
                                    // a greyed preset still explains itself; the
                                    // primitive keeps it non-selectable regardless.
                                    className={
                                        reasonFor(o.value)
                                            ? "data-disabled:pointer-events-auto data-disabled:cursor-not-allowed"
                                            : undefined
                                    }
                                >
                                    <OptionRow
                                        option={o}
                                        disabledReason={reasonFor(o.value)}
                                    />
                                </SelectItem>
                            ))}
                        </SelectGroup>
                        {saved.length > 0 && (
                            <>
                                <SelectSeparator />
                                <SelectGroup>
                                    <SelectLabel>Saved presets</SelectLabel>
                                    {saved.map(o => (
                                        <SelectItem key={o.value} value={o.value}>
                                            <OptionRow option={o} />
                                        </SelectItem>
                                    ))}
                                </SelectGroup>
                            </>
                        )}
                    </SelectContent>
                </Select>
                {isUserPreset(preset) && (
                    <Button
                        variant="ghost"
                        size="icon"
                        aria-label="Delete preset"
                        disabled={publishing}
                        onClick={() => onDeletePreset(preset)}
                    >
                        <IconTrash />
                    </Button>
                )}
                <span className="text-sm text-muted-foreground">
                    {presetHint(preset)}
                </span>
            </CardContent>
        </Card>
    );
}
