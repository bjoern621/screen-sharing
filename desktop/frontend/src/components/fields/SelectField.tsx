import {
    Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import { Option } from "../../types/stream";
import { labelFor } from "../../util/options";
import OptionRow from "../OptionRow/OptionRow";
import FieldShell from "./FieldShell";

interface SelectFieldProps {
    label: string;
    labelTip: string;
    value: string;
    options: Option[];
    /** Reason the whole control is ignored; disables and greys it out. */
    disabledReason?: string;
    /** value -> reason a single option is unavailable. */
    optionDisabled?: Record<string, string>;
    onChange: (value: string) => void;
}

/**
 * A dependency-aware dropdown. The trigger renders the same label as the option
 * row so both always read identically. Individual options can be disabled with
 * a reason without disabling the whole control.
 */
export default function SelectField({
    label,
    labelTip,
    value,
    options,
    disabledReason,
    optionDisabled,
    onChange,
}: SelectFieldProps) {
    return (
        <FieldShell label={label} labelTip={labelTip} disabledReason={disabledReason}>
            <Select
                value={value}
                disabled={!!disabledReason}
                onValueChange={(v: string | null) => v && onChange(v)}
            >
                <SelectTrigger className="w-full">
                    <SelectValue>{(v: string) => labelFor(options, v)}</SelectValue>
                </SelectTrigger>
                <SelectContent>
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
        </FieldShell>
    );
}
