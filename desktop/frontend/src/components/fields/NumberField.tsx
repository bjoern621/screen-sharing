import { Input } from "@/components/ui/input";
import FieldShell from "./FieldShell";

interface NumberFieldProps {
    label: string;
    labelTip?: string;
    labelLink?: string;
    value: number;
    /** Range the encoder accepts, when the value has one. The spinner stops there
     * and the browser marks a typed value outside it invalid. */
    min?: number;
    max?: number;
    disabledReason?: string;
    onChange: (value: number) => void;
}

/** Integer input wrapped in the shared field shell. */
export default function NumberField({
    label,
    labelTip,
    labelLink,
    value,
    min,
    max,
    disabledReason,
    onChange,
}: NumberFieldProps) {
    return (
        <FieldShell label={label} labelTip={labelTip} labelLink={labelLink} disabledReason={disabledReason}>
            <Input
                type="number"
                value={value}
                min={min}
                max={max}
                disabled={!!disabledReason}
                onChange={e => onChange(parseInt(e.target.value, 10) || 0)}
            />
        </FieldShell>
    );
}
