import { Button as ButtonPrimitive } from "@base-ui/react/button"
import { cva, type VariantProps } from "class-variance-authority"

import { cn, disabledControl } from "@/lib/utils"

/*
 * Every variant states the same three things in the same order: the resting
 * fill, what hover does to it, and what press does to it. A variant that names
 * only two of them is the defect this shape exists to make visible.
 *
 * The fill and the lift are guarded by `not-disabled:`. Disabled buttons
 * deliberately keep pointer events so the not-allowed cursor shows (see
 * `disabledControl`), which means a bare `hover:` would still repaint a control
 * that cannot be pressed - the one hover state that lies.
 *
 * Hover text colour is deliberately left unguarded on the variants that have a
 * fill. `not-disabled:hover:text-*` outranks a plain `hover:text-*` on
 * specificity, and tailwind-merge cannot collapse the two because they are
 * different variant chains, so guarding it would silently beat the call sites
 * that keep an icon button red through its own hover (AudioOnlyChip, Tile). A
 * label shifting one step under a pointer that cannot press it is a much smaller
 * lie than a fill lighting up. link is the exception: its colour is its fill,
 * so that one is guarded.
 *
 * Fills come from the -hover/-active tokens in index.css rather than from an
 * opacity step. A translucent fill lets the surface behind it through, so the
 * same button reads one colour on a card and another over a video tile; the
 * tokens mix in colour space and stay opaque, so it reads the same everywhere.
 * box-shadow is in the transition list although no variant carries one: the
 * design keeps elevation to the window and the context menu, and a button
 * handed a shadow at a call site should still animate it.
 */
const buttonVariants = cva(
  `group/button inline-flex shrink-0 items-center justify-center rounded-md border border-transparent bg-clip-padding text-xs/relaxed font-medium whitespace-nowrap outline-none select-none transition-[color,background-color,border-color,box-shadow,opacity,transform] focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/30 not-disabled:cursor-pointer not-disabled:active:not-aria-[haspopup]:translate-y-px ${disabledControl} aria-invalid:border-destructive aria-invalid:ring-2 aria-invalid:ring-destructive/20 dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4`,
  {
    variants: {
      variant: {
        default:
          "bg-primary text-primary-foreground not-disabled:hover:bg-primary-hover not-disabled:active:bg-primary-active",
        outline:
          "border-border aria-expanded:bg-muted aria-expanded:text-foreground dark:bg-input/30 not-disabled:hover:border-ring/60 not-disabled:hover:bg-muted-hover hover:text-foreground not-disabled:active:bg-muted-active",
        secondary:
          "bg-secondary text-secondary-foreground aria-expanded:bg-secondary aria-expanded:text-secondary-foreground not-disabled:hover:bg-secondary-hover not-disabled:active:bg-secondary-active",
        // A filled button that recedes until it is pointed at. --muted and
        // --secondary hold the same grey, so the fill is not what separates this
        // from secondary: the label is. It rests at muted-foreground and comes up
        // to full foreground on hover, which is the whole state change. A variant
        // that only repeated secondary's fill under a second name would be worth
        // nothing.
        muted:
          "bg-muted text-muted-foreground not-disabled:hover:bg-muted-hover hover:text-foreground not-disabled:active:bg-muted-active",
        // The one raised control. Elevation grows on hover and is gone on press,
        // at the same moment the base template drops the button a pixel, so the
        // press reads as the button meeting the surface. See --shadow-raised in
        // index.css for why this exists on this surface and nowhere else.
        shadow:
          "bg-secondary text-secondary-foreground shadow-raised not-disabled:hover:bg-secondary-hover not-disabled:hover:shadow-raised-hover not-disabled:active:bg-secondary-active not-disabled:active:shadow-none",
        ghost:
          "aria-expanded:bg-muted aria-expanded:text-foreground not-disabled:hover:bg-muted-hover hover:text-foreground not-disabled:active:bg-muted-active",
        destructive:
          "bg-destructive/10 text-destructive focus-visible:border-destructive/40 focus-visible:ring-destructive/20 dark:bg-destructive/20 dark:focus-visible:ring-destructive/40 not-disabled:hover:bg-destructive/20 not-disabled:active:bg-destructive/30 dark:not-disabled:hover:bg-destructive/30 dark:not-disabled:active:bg-destructive/40",
        link: "text-primary underline-offset-4 not-disabled:hover:text-primary-hover not-disabled:hover:underline not-disabled:active:text-primary-active",
      },
      size: {
        default:
          "h-7 gap-1 px-2 text-xs/relaxed has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3.5",
        xs: "h-5 gap-1 rounded-sm px-2 text-[0.625rem] has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-2.5",
        sm: "h-6 gap-1 px-2 text-xs/relaxed has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3",
        lg: "h-8 gap-1 px-2.5 text-xs/relaxed has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2 [&_svg:not([class*='size-'])]:size-4",
        icon: "size-7 [&_svg:not([class*='size-'])]:size-3.5",
        "icon-xs": "size-5 rounded-sm [&_svg:not([class*='size-'])]:size-2.5",
        "icon-sm": "size-6 [&_svg:not([class*='size-'])]:size-3",
        "icon-lg": "size-8 [&_svg:not([class*='size-'])]:size-4",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

function Button({
  className,
  variant = "default",
  size = "default",
  ...props
}: ButtonPrimitive.Props & VariantProps<typeof buttonVariants>) {
  return (
    <ButtonPrimitive
      data-slot="button"
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  )
}

export { Button, buttonVariants }
