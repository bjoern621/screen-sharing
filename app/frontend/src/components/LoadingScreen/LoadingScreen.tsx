import { IconLoader2 } from "@tabler/icons-react";

/** Full-viewport spinner shown while the initial settings load. */
export default function LoadingScreen() {
    return (
        <div className="flex h-screen items-center justify-center">
            <IconLoader2 size={32} className="animate-spin text-muted-foreground" />
        </div>
    );
}
