import { Input } from "@/components/ui/input";
import FieldShell from "./FieldShell";

interface TextFieldProps {
    label: string;
    labelTip: string;
    value: string;
    onChange: (value: string) => void;
}

/** Free-text input wrapped in the shared field shell. */
export default function TextField({
    label,
    labelTip,
    value,
    onChange,
}: TextFieldProps) {
    return (
        <FieldShell label={label} labelTip={labelTip}>
            <Input value={value} onChange={e => onChange(e.target.value)} />
        </FieldShell>
    );
}
