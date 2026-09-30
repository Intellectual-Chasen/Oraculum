import { type ReactNode, useEffect, useId, useRef, useState } from "react";
import { Dialog, DialogTrigger, Heading, Popover } from "react-aria-components";
import { Button } from "./Button";
import { usePortalContainer } from "./portalContainer";

/** 長い指定を表の外で編集する。ファイルから読む場合も、適用する前に内容を確認できる。 */
export function TextEditorPopover({
  label,
  value,
  inputLabel,
  description,
  target,
  applyToGroupLabel,
  onCommit,
}: {
  label: string;
  value: string;
  inputLabel: string;
  description: ReactNode;
  target: ReactNode;
  applyToGroupLabel?: string;
  onCommit: (value: string, applyToGroup: boolean) => void;
}) {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState(value);
  const [applyToGroup, setApplyToGroup] = useState(false);
  const [failure, setFailure] = useState("");
  const [isReading, setIsReading] = useState(false);
  const readVersion = useRef(0);
  useEffect(
    () => () => {
      readVersion.current++;
    },
    [],
  );
  const descriptionId = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const portal = usePortalContainer();
  const changeOpen = (next: boolean) => {
    readVersion.current++;
    setIsReading(false);
    if (next) {
      setDraft(value);
      setApplyToGroup(false);
      setFailure("");
    }
    setOpen(next);
  };
  return (
    <DialogTrigger isOpen={open} onOpenChange={changeOpen}>
      <Button variant="secondary" aria-label={label}>
        {value.trim() === "" ? `${inputLabel}を指定` : `${inputLabel}を編集`}
      </Button>
      <Popover
        placement="bottom start"
        UNSTABLE_portalContainer={portal}
        className="text-editor-popover rounded-md border border-line bg-surface p-4 shadow-float"
      >
        <Dialog className="flex flex-col gap-3 outline-none">
          <Heading slot="title" className="text-md font-medium">
            {inputLabel}の指定
          </Heading>
          <div className="text-editor-target">{target}</div>
          <p id={descriptionId} className="text-xs text-muted">
            {description}
          </p>
          <label className="text-editor-value">
            {inputLabel}
            <textarea
              aria-label={inputLabel}
              aria-describedby={descriptionId}
              value={draft}
              rows={6}
              onChange={(event) => {
                readVersion.current++;
                setIsReading(false);
                setFailure("");
                setDraft(event.target.value);
              }}
            />
          </label>
          <input
            ref={inputRef}
            type="file"
            hidden
            aria-label="書式のテキストファイル"
            onChange={async (event) => {
              const file = event.currentTarget.files?.[0];
              event.currentTarget.value = "";
              if (!file) return;
              const version = ++readVersion.current;
              setIsReading(true);
              setFailure("");
              try {
                const content = await file.text();
                if (version === readVersion.current) setDraft(content);
              } catch {
                if (version === readVersion.current)
                  setFailure("書式ファイルを読めませんでした");
              } finally {
                if (version === readVersion.current) setIsReading(false);
              }
            }}
          />
          <Button variant="secondary" onPress={() => inputRef.current?.click()}>
            書式ファイルを選択
          </Button>
          {failure === "" ? null : <p role="alert">{failure}</p>}
          {isReading ? <p role="status">書式ファイルを読み込み中</p> : null}
          {applyToGroupLabel === undefined ? null : (
            <label className="text-editor-apply">
              <input
                type="checkbox"
                checked={applyToGroup}
                onChange={(event) => setApplyToGroup(event.target.checked)}
              />
              {applyToGroupLabel}
            </label>
          )}
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onPress={() => changeOpen(false)}>
              取消
            </Button>
            <Button
              variant="primary"
              isDisabled={isReading || draft.trim() === ""}
              disabledReason={{
                title: isReading ? "書式ファイルを読み込み中" : "入力が必要",
                text: isReading
                  ? "ファイルの読み込み完了後に適用できます"
                  : `${inputLabel}を入力してください`,
              }}
              onPress={() => {
                onCommit(draft, applyToGroup);
                changeOpen(false);
              }}
            >
              適用
            </Button>
          </div>
        </Dialog>
      </Popover>
    </DialogTrigger>
  );
}
