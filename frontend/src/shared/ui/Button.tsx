import { useId } from "react";
import {
  Button as AriaButton,
  type ButtonProps as AriaButtonProps,
  composeRenderProps,
} from "react-aria-components";
import { tv, type VariantProps } from "tailwind-variants";
import { Tooltip, TooltipTrigger } from "./Tooltip";

export const buttonVariants = tv({
  base: [
    "inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-sm font-medium",
    "cursor-default transition-colors outline-none",
    "data-[focus-visible]:outline-2 data-[focus-visible]:outline-offset-2 data-[focus-visible]:outline-accent",
  ],
  variants: {
    // 高さはどの大きさも入力欄・select と同じ --control-height にする。sm は文字と左右の余白だけを小さくする。
    size: {
      md: "h-(--control-height) px-3 text-sm",
      sm: "h-(--control-height) px-2 text-xs",
      icon: "size-(--control-height) p-0",
      iconSm: "size-(--control-height) p-0",
    },
    variant: {
      primary:
        "bg-accent text-accent-ink data-[hovered]:bg-accent-hover data-[pressed]:bg-accent-pressed",
      secondary:
        "border border-line bg-surface text-ink data-[hovered]:bg-ground data-[pressed]:bg-line",
      ghost:
        "text-muted data-[hovered]:bg-ground data-[hovered]:text-ink data-[pressed]:bg-line",
      danger:
        "border border-danger text-danger data-[hovered]:bg-danger-soft data-[pressed]:bg-danger-soft",
    },
    // 押せない状態は、文字が読める濃さの淡い色で示す。hover と focus は受け、理由の tooltip を出す。
    unavailable: {
      true: "border-line bg-ground text-faint data-[hovered]:bg-ground data-[hovered]:text-faint data-[pressed]:bg-ground",
    },
    fullWidth: { true: "w-full" },
  },
  defaultVariants: { size: "md", variant: "secondary" },
});

export type ButtonVariants = VariantProps<typeof buttonVariants>;

/** 押せない理由。見出しは短い結論、文は押せるようにする条件を書く。 */
export type DisabledReason = { title: string; text: string };

export type ButtonProps = Omit<AriaButtonProps, "className"> &
  Omit<ButtonVariants, "unavailable"> & {
    className?: string;
    /**
     * 押せない理由。`isDisabled` と一緒に渡すと、button は focus を受けたまま押せなくなり、
     * hover と focus で理由を出す。渡さない `isDisabled` は、focus も受けない。
     */
    disabledReason?: DisabledReason;
    /** 押せるときに hover と focus で出す tooltip。押せないときは `disabledReason` を出す。 */
    tooltip?: string;
  };

/**
 * React Aria の Button を包んだ button。keyboard・pointer・読み上げの操作は React Aria が扱う。
 */
export function Button({
  size,
  variant,
  fullWidth,
  className,
  disabledReason,
  tooltip,
  isDisabled,
  onPress,
  ...props
}: ButtonProps) {
  const unavailable = isDisabled === true && disabledReason !== undefined;
  const reasonId = useId();
  const classNameOf = (extra?: string) =>
    buttonVariants({
      size,
      variant,
      fullWidth,
      unavailable: unavailable || isDisabled === true,
      className: extra,
    });
  const button = (
    <AriaButton
      {...props}
      isDisabled={isDisabled === true && !unavailable}
      onPress={unavailable ? undefined : onPress}
      // 押せない button は disabled を付けないため、送信の button のままだと click と Enter で
      // form を送る。送信の button にせず、click の既定の動作も止める。
      type={unavailable ? "button" : props.type}
      onClick={unavailable ? (event) => event.preventDefault() : undefined}
      aria-disabled={unavailable ? true : undefined}
      aria-describedby={
        unavailable
          ? [props["aria-describedby"], reasonId].filter(Boolean).join(" ")
          : props["aria-describedby"]
      }
      className={composeRenderProps(className, (extra) => classNameOf(extra))}
    />
  );
  if (disabledReason === undefined && tooltip === undefined) {
    return button;
  }
  // **押せる状態と押せない状態で要素の木の形を変えない。** 形が変わると React が button を
  // 作り直し、focus が外れる。tooltip の中身だけを状態で替える。
  // 理由は tooltip が開いていなくても読み上げに届くよう、画面に出さない文としても結び付ける。
  // 別ウィンドウに出した区画では、keyboard の focus で tooltip が開かないことがある。
  return (
    <>
      <TooltipTrigger
        delay={300}
        isDisabled={!unavailable && tooltip === undefined}
      >
        {button}
        {unavailable ? (
          <Tooltip title={disabledReason.title}>{disabledReason.text}</Tooltip>
        ) : (
          <Tooltip>{tooltip}</Tooltip>
        )}
      </TooltipTrigger>
      {unavailable ? (
        <span id={reasonId} className="sr-only">
          {`${disabledReason.title}: ${disabledReason.text}`}
        </span>
      ) : null}
    </>
  );
}
