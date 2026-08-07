import { mergeProps } from "@base-ui/react/merge-props"
import { useRender } from "@base-ui/react/use-render"
import { cva, type VariantProps } from "class-variance-authority"

import { cn } from "@/lib/utils"

/*
 * A badge is a label, not a control. Most of it reports state - a transport
 * name, a drop count, "n watching" - and reacting to a pointer that cannot do
 * anything with it is the hover state that teaches the wrong thing. So every
 * hover here is gated on `[a]:`: a badge earns one only once a call site renders
 * it as a link. ghost and link used to hover unconditionally and are gated the
 * same way now.
 *
 * The fills come from the same -hover tokens the button uses, so a badge
 * rendered as a link and a button placed beside it move by the same step.
 */
const badgeVariants = cva(
  "group/badge inline-flex h-5 w-fit shrink-0 items-center justify-center gap-1 overflow-hidden rounded-full border border-transparent px-2 py-0.5 text-[0.625rem] font-medium whitespace-nowrap transition-[color,background-color,border-color,box-shadow] focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/30 has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 [a]:cursor-pointer [&>svg]:pointer-events-none [&>svg]:size-2.5!",
  {
    variants: {
      variant: {
        default:
          "bg-primary text-primary-foreground [a]:hover:bg-primary-hover [a]:active:bg-primary-active",
        secondary:
          "bg-secondary text-secondary-foreground [a]:hover:bg-secondary-hover [a]:active:bg-secondary-active",
        destructive:
          "bg-destructive/10 text-destructive focus-visible:ring-destructive/20 dark:bg-destructive/20 dark:focus-visible:ring-destructive/40 [a]:hover:bg-destructive/20 [a]:active:bg-destructive/30 dark:[a]:hover:bg-destructive/30 dark:[a]:active:bg-destructive/40",
        outline:
          "border-border bg-input/20 text-foreground dark:bg-input/30 [a]:hover:border-ring/60 [a]:hover:bg-muted-hover [a]:hover:text-foreground [a]:active:bg-muted-active",
        ghost:
          "[a]:hover:bg-muted-hover [a]:hover:text-foreground [a]:active:bg-muted-active",
        link: "text-primary underline-offset-4 [a]:hover:text-primary-hover [a]:hover:underline [a]:active:text-primary-active",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
)

function Badge({
  className,
  variant = "default",
  render,
  ...props
}: useRender.ComponentProps<"span"> & VariantProps<typeof badgeVariants>) {
  return useRender({
    defaultTagName: "span",
    props: mergeProps<"span">(
      {
        className: cn(badgeVariants({ variant }), className),
      },
      props
    ),
    render,
    state: {
      slot: "badge",
      variant,
    },
  })
}

export { Badge, badgeVariants }
