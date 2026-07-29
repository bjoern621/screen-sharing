import { IconArrowBackUp, IconRefresh } from "@tabler/icons-react";
import { Button } from "@/components/ui/button";
import Tip from "../Tip/Tip";

interface PublishPendingProps {
    onApply: () => void;
    onRevert: () => void;
}

/**
 * The bar shown while the live stream runs on settings the form has moved off. It
 * carries the two ways out: send the form's settings to the stream, or take the form
 * back to the stream's.
 */
export default function PublishPending({
    onApply,
    onRevert,
}: PublishPendingProps) {
    return (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-md border bg-muted/50 px-3 py-2">
            <p className="text-sm">
                The live stream still runs on the settings it was started on.
            </p>
            <div className="flex items-center gap-2">
                <Tip text="Restarts the publish pipeline on the settings shown here. Both publish engines run a command line, so the values reach the stream as a new pipeline: the stream drops for about a second and viewers reconnect to it.">
                    <Button size="sm" onClick={onApply}>
                        <IconRefresh size={16} />
                        Apply to live stream
                    </Button>
                </Tip>
                <Tip text="Puts the settings the live stream is running back into the form. The stream is left alone; this only takes the form back to what the viewers are watching.">
                    <Button variant="outline" size="sm" onClick={onRevert}>
                        <IconArrowBackUp size={16} />
                        Revert to live settings
                    </Button>
                </Tip>
            </div>
        </div>
    );
}
