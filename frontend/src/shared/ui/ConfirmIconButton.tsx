import type { ComponentProps, ReactNode } from "react";
import { Dialog, DialogTrigger, Heading, Popover } from "react-aria-components";
import { Button } from "./Button";
import { IconButton } from "./IconButton";
import { usePortalContainer } from "./portalContainer";

/**
 * 取り消せない操作の IconButton。押すと確認の dialog を開き、dialog の「`label`」を押したときだけ
 * `onConfirm` を呼ぶ。`details` は操作の対象を「名前: 値」の組で渡す。
 *
 * 押せないときは dialog を開かず、IconButton の `disabledReason` を出す。
 */
export function ConfirmIconButton({
  label,
  details,
  onConfirm,
  children,
  ...props
}: Omit<ComponentProps<typeof IconButton>, "onPress"> & {
  details?: ReactNode;
  onConfirm: () => void;
}) {
  const portalContainer = usePortalContainer();
  if (props.isDisabled === true) {
    return (
      <IconButton {...props} label={label}>
        {children}
      </IconButton>
    );
  }
  return (
    <DialogTrigger>
      <IconButton {...props} label={label}>
        {children}
      </IconButton>
      <Popover
        UNSTABLE_portalContainer={portalContainer}
        placement="bottom end"
        className="min-w-56 rounded-md border border-line bg-surface p-3 text-sm shadow-float outline-none"
      >
        <Dialog role="alertdialog" className="flex flex-col gap-2 outline-none">
          {({ close }) => (
            <>
              <Heading slot="title" className="text-sm font-medium">
                {label}
              </Heading>
              {details}
              <div className="flex justify-end gap-2">
                <Button size="sm" onPress={close}>
                  取消
                </Button>
                <Button
                  size="sm"
                  variant="primary"
                  onPress={() => {
                    close();
                    onConfirm();
                  }}
                >
                  {label}
                </Button>
              </div>
            </>
          )}
        </Dialog>
      </Popover>
    </DialogTrigger>
  );
}
