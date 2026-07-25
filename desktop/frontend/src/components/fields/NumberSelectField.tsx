import {
    Select, SelectContent, SelectItem, SelectTrigger,
} from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { Option } from "../../types/stream";
import OptionRow from "../OptionRow/OptionRow";
import FieldShell from "./FieldShell";

interface NumberSelectFieldProps {
    label: string;
    labelTip?: string;
    value: number;
    /** Preset values offered by the attached dropdown. */
    options: Option[];
    /** Range accepted for a typed value. The spinner stops there and the browser
     * marks a typed value outside it invalid. */
    min?: number;
    max?: number;
    /** Reason the whole control is ignored; disables and greys it out. */
    disabledReason?: string;
    /** value -> reason a single preset is unavailable. A typed value bypasses
     * this, so a disabled preset is a recommendation, not a hard limit. */
    optionDisabled?: Record<string, string>;
    onChange: (value: number) => void;
}

/**
 * Numeric field with a preset dropdown attached to its trailing edge. The input
 * and the dropdown write the same value, so a preset is a shortcut and any other
 * value can be typed. Presets outside what the current dependencies support are
 * disabled with a reason; typing one anyway is allowed.
 */
export default function NumberSelectField({
    label,
    labelTip,
    value,
    options,
    min,
    max,
    disabledReason,
    optionDisabled,
    onChange,
}: NumberSelectFieldProps) {
    const disabled = !!disabledReason;
    return (
        <FieldShell label={label} labelTip={labelTip} disabledReason={disabledReason}>
            <div className="flex">
                <Input
                    type="number"
                    className="rounded-r-none"
                    value={value}
                    min={min}
                    max={max}
                    disabled={disabled}
                    onChange={e => onChange(parseInt(e.target.value, 10) || 0)}
                />
                <Select
                    value={String(value)}
                    disabled={disabled}
                    onValueChange={(v: string | null) => v && onChange(parseInt(v, 10) || 0)}
                >
                    {/* Value-less trigger: the number lives in the input, so the
                     * dropdown is a chevron that only picks presets. */}
                    <SelectTrigger
                        aria-label={`${label} presets`}
                        className="-ml-px rounded-l-none px-1.5"
                    />
                    <SelectContent align="end">
                        {options.map(o => {
                            const reason = optionDisabled?.[o.value];
                            return (
                                <SelectItem
                                    key={o.value}
                                    value={o.value}
                                    disabled={!!reason}
                                    // Base UI sets pointer-events-none on disabled items,
                                    // which would swallow the hover that opens the option's
                                    // tooltip. Restore pointer events so a disabled option
                                    // still explains why it is unavailable; the primitive
                                    // keeps it non-selectable regardless.
                                    className={
                                        reason
                                            ? "data-disabled:pointer-events-auto data-disabled:cursor-not-allowed"
                                            : undefined
                                    }
                                >
                                    <OptionRow option={o} disabledReason={reason} />
                                </SelectItem>
                            );
                        })}
                    </SelectContent>
                </Select>
            </div>
        </FieldShell>
    );
}
