import * as React from "react"
import { Input as InputPrimitive } from "@base-ui/react/input"

import { cn, disabledControl } from "@/lib/utils"

function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return (
    <InputPrimitive
      type={type}
      data-slot="input"
      className={cn(
        // Hover moves the border and the fill by one step, the same step the
        // select trigger takes, so the two controls sitting in one row answer
        // the pointer identically. Focus still wins over hover: Tailwind emits
        // focus-visible after hover, so the ring border replaces the hover one
        // rather than competing with it.
        "h-7 w-full min-w-0 rounded-md border border-input bg-input/20 px-2 py-0.5 text-sm transition-[color,background-color,border-color,box-shadow] outline-none file:inline-flex file:h-6 file:border-0 file:bg-transparent file:text-xs/relaxed file:font-medium file:text-foreground placeholder:text-muted-foreground not-disabled:hover:border-ring/60 not-disabled:hover:bg-input/40 focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/30 aria-invalid:border-destructive aria-invalid:ring-2 aria-invalid:ring-destructive/20 md:text-xs/relaxed dark:bg-input/30 dark:not-disabled:hover:bg-input/50 dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40",
        disabledControl,
        className
      )}
      {...props}
    />
  )
}

export { Input }
