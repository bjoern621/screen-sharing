import { Option } from "../../types/stream";
import Tip from "../Tip/Tip";
import InfoIcon from "../InfoIcon/InfoIcon";

interface OptionRowProps {
    option: Option;
    disabledReason?: string;
}

// The trigger fills the row, whose right edge sits ~pr-8 (32px) inside the
// popup; clear the popup's right border plus a small gap.
const TOOLTIP_SIDE_OFFSET = 44;

/**
 * Renders one select option: label plus an optional reference-article icon,
 * wrapped in a tooltip pinned to the side so it never covers other options.
 */
export default function OptionRow({ option, disabledReason }: OptionRowProps) {
    return (
        <Tip
            text={disabledReason ? `Unavailable: ${disabledReason}` : option.tip}
            side="right"
            sideOffset={TOOLTIP_SIDE_OFFSET}
            className="flex w-full items-center gap-2"
        >
            <span>{option.label}</span>
            {option.link && <InfoIcon url={option.link} />}
        </Tip>
    );
}
