import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * Disabled-state styling every form control shares (input, button, select
 * trigger) so a disabled control always reads the same: dimmed with a
 * not-allowed cursor. pointer-events-none is deliberately excluded; it
 * suppresses the cursor, so a control carrying it shows the default arrow
 * instead of not-allowed. The native disabled attribute already blocks
 * interaction, so nothing is needed to stop clicks.
 */
export const disabledControl =
  "disabled:cursor-not-allowed disabled:opacity-50"
