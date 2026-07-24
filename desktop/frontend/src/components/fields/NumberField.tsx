import { Input } from "@/components/ui/input";
import FieldShell from "./FieldShell";

interface NumberFieldProps {
    label: string;
    labelTip?: string;
    value: number;
    disabledReason?: string;
    onChange: (value: number) => void;
}

/** Integer input wrapped in the shared field shell. */
export default function NumberField({
    label,
    labelTip,
    value,
    disabledReason,
    onChange,
}: NumberFieldProps) {
    return (
        <FieldShell label={label} labelTip={labelTip} disabledReason={disabledReason}>
            <Input
                type="number"
                value={value}
                disabled={!!disabledReason}
                onChange={e => onChange(parseInt(e.target.value, 10) || 0)}
            />
        </FieldShell>
    );
}
