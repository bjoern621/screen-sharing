import { Slider as SliderPrimitive } from "@base-ui/react/slider"

import { cn } from "@/lib/utils"

function Slider({
  className,
  defaultValue,
  value,
  min = 0,
  max = 100,
  ...props
}: SliderPrimitive.Root.Props) {
  const _values = Array.isArray(value)
    ? value
    : Array.isArray(defaultValue)
      ? defaultValue
      : [min, max]

  return (
    <SliderPrimitive.Root
      className={cn(
        // The not-allowed cursor sits on the root rather than on the control,
        // because the control below drops pointer events while disabled so that
        // none of its hover states can fire. The root keeps them, so the cursor
        // still reports why nothing happens.
        "data-[orientation=horizontal]:w-full data-[orientation=vertical]:h-full data-disabled:cursor-not-allowed",
        className
      )}
      data-slot="slider"
      defaultValue={defaultValue}
      value={value}
      min={min}
      max={max}
      {...props}
    >
      {/* The whole control is one hover target, not three. Pointing anywhere on
          the bar lights the track, the fill and the thumb together, so a 4px
          track in a crowded tile chrome still announces that it is draggable
          before the pointer has found the thumb. */}
      <SliderPrimitive.Control className="group/slider relative flex w-full touch-none items-center select-none data-disabled:pointer-events-none data-disabled:opacity-50 data-[orientation=vertical]:h-full data-[orientation=vertical]:min-h-40 data-[orientation=vertical]:w-auto data-[orientation=vertical]:flex-col">
        <SliderPrimitive.Track
          data-slot="slider-track"
          className="relative grow overflow-hidden rounded-md bg-muted transition-colors select-none group-hover/slider:bg-muted-hover group-data-dragging/slider:bg-muted-hover data-[orientation=horizontal]:h-1 data-[orientation=horizontal]:w-full data-[orientation=vertical]:h-full data-[orientation=vertical]:w-1"
        >
          <SliderPrimitive.Indicator
            data-slot="slider-range"
            className="bg-primary transition-colors select-none group-hover/slider:bg-primary-hover group-data-dragging/slider:bg-primary-hover data-[orientation=horizontal]:h-full data-[orientation=vertical]:w-full"
          />
        </SliderPrimitive.Track>
        {Array.from({ length: _values.length }, (_, index) => (
          // The thumb grows on hover and settles back a little under the
          // pointer while dragging, which is the whole press feedback: the
          // handle is 12px and has no room for a fill change that reads.
          // Base UI centres it with the `translate` property rather than with
          // `transform`, so a scale here composes with it instead of fighting
          // it.
          <SliderPrimitive.Thumb
            data-slot="slider-thumb"
            key={index}
            className="relative block size-3 shrink-0 cursor-grab rounded-md border border-ring bg-white ring-ring/30 transition-[transform,box-shadow,border-color] select-none after:absolute after:-inset-2 hover:scale-125 hover:ring-2 focus-visible:scale-125 focus-visible:ring-2 focus-visible:outline-hidden active:scale-110 active:cursor-grabbing active:ring-2 data-dragging:scale-110 data-dragging:cursor-grabbing data-dragging:ring-2"
          />
        ))}
      </SliderPrimitive.Control>
    </SliderPrimitive.Root>
  )
}

export { Slider }
