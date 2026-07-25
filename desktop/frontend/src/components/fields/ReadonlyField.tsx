import FieldShell from "./FieldShell";

interface ReadonlyFieldProps {
    label: string;
    labelTip?: string;
    /** The derived value, already formatted for display. */
    value: string;
}

/**
 * A field that shows a fact rather than offering a choice: same label, tooltip and
 * row height as the editable fields, no control. Used where a value follows from
 * another setting and picking it separately would be a second source of truth, as the
 * publish engine follows from the capture backend.
 *
 * The value carries no input border, so the row reads as a statement and not as a
 * control that happens to be disabled.
 */
export default function ReadonlyField({
    label,
    labelTip,
    value,
}: ReadonlyFieldProps) {
    return (
        <FieldShell label={label} labelTip={labelTip}>
            <p className="flex h-7 items-center text-xs/relaxed font-medium">
                {value}
            </p>
        </FieldShell>
    );
}
