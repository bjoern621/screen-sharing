import { IconRefreshAlert } from "@tabler/icons-react";

interface PublishRetryingProps {
    attempt: number;
    budget: number;
}

/**
 * The bar shown while a publish that died on its own waits to be started again. It
 * carries no control: the stop button beside it already ends the attempts, and the
 * settings above it are the ones the next one will run.
 */
export default function PublishRetrying({
    attempt,
    budget,
}: PublishRetryingProps) {
    return (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2">
            <IconRefreshAlert size={16} className="animate-spin text-amber-600" />
            <p className="text-sm">
                The publish pipeline ended on its own. Starting it again, attempt{" "}
                {attempt} of {budget}.
            </p>
        </div>
    );
}
