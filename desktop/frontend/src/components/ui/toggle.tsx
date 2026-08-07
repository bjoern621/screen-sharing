import { Toggle as TogglePrimitive } from "@base-ui/react/toggle"
import { cva, type VariantProps } from "class-variance-authority"

import { cn } from "@/lib/utils"

/*
 * A toggle is on or off, and it is also hovered or not, so the two have to stay
 * separable: an on toggle brightens further under the pointer instead of sitting
 * at the same fill as an off one being hovered. The pressed-state classes are
 * stacked on the on-state ones (`aria-pressed:hover:`) so they outrank the plain
 * on-state fill by specificity rather than by whatever order Tailwind happens to
 * emit the two variants in. The price is that a call site giving a toggle its own
 * on-fill has to give it an on-hover as well, or this one wins and greys it out
 * under the pointer - StreamRoster's chip is the one place that does.
 *
 * The focus ring matches the button's - 2px at 30% - so tabbing across a row of
 * mixed controls does not change the shape of the ring that follows the caret.
 */
const toggleVariants = cva(
  "group/toggle inline-flex cursor-pointer items-center justify-center gap-1 rounded-md text-xs font-medium whitespace-nowrap outline-none transition-[color,background-color,border-color,box-shadow,transform] hover:bg-muted-hover hover:text-foreground focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/30 active:translate-y-px active:bg-muted-active disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 aria-pressed:bg-muted aria-pressed:hover:bg-muted-hover data-[state=on]:bg-muted data-[state=on]:hover:bg-muted-hover dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default: "bg-transparent",
        outline:
          "border border-input bg-transparent hover:border-ring/60 hover:bg-muted-hover",
      },
      size: {
        default:
          "h-7 min-w-7 px-2 has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5",
        sm: "h-6 min-w-6 rounded-[min(var(--radius-md),8px)] px-2 text-[0.625rem] has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3",
        lg: "h-8 min-w-8 px-2.5 has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

function Toggle({
  className,
  variant = "default",
  size = "default",
  ...props
}: TogglePrimitive.Props & VariantProps<typeof toggleVariants>) {
  return (
    <TogglePrimitive
      data-slot="toggle"
      className={cn(toggleVariants({ variant, size, className }))}
      {...props}
    />
  )
}

export { Toggle, toggleVariants }
