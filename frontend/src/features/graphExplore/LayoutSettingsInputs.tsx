import { SlidersHorizontal } from "lucide-react";
import { Dialog, DialogTrigger, Heading, Popover } from "react-aria-components";
import { Button } from "@/shared/ui/Button";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { usePortalContainer } from "@/shared/ui/portalContainer";
import { type CosmosLayoutSettings, layoutSettingRanges } from "./cosmosLayout";

/** 配置の設定ごとの、入力欄の名前と、値を大きくしたときの図の変化。 */
const settingLabels: Record<
  keyof CosmosLayoutSettings,
  { label: string; larger: string }
> = {
  simulationGravity: {
    label: "中心へ寄せる強さ",
    larger: "中心に集まるノード",
  },
  simulationLinkDistance: {
    label: "エッジの長さ",
    larger: "離れるノード",
  },
  simulationCluster: {
    label: "互いに多く繋がったノードを寄せる強さ",
    larger: "互いに多く繋がったノードの集まり",
  },
};

/**
 * 図の配置の設定の入力欄。
 * 範囲の外の値と数でない値は受け取らず、直前の値を保つ。欄から離れると、欄の表示を
 * 使っている値に戻す。
 */
export function LayoutSettingsInputs({
  value,
  onChange,
}: {
  value: CosmosLayoutSettings;
  onChange: (settings: CosmosLayoutSettings) => void;
}) {
  return (
    <>
      {(Object.keys(layoutSettingRanges) as (keyof CosmosLayoutSettings)[]).map(
        (name) => {
          const range = layoutSettingRanges[name];
          const id = `layout-setting-${name}`;
          return (
            <div key={name} className="flex flex-col gap-0.5">
              <label htmlFor={id} className="text-sm font-medium text-ink">
                {settingLabels[name].label}
              </label>
              <input
                id={id}
                type="number"
                min={range.min}
                max={range.max}
                step={range.step}
                aria-describedby={`${id}-description`}
                className="w-28 rounded-sm border border-line bg-surface px-2 font-mono text-base"
                // 入力の途中の値 (空欄など) を消さないよう、欄の文字は入力欄が持つ。
                defaultValue={value[name]}
                onChange={(event) => {
                  const next = event.target.valueAsNumber;
                  if (
                    Number.isFinite(next) &&
                    next >= range.min &&
                    next <= range.max
                  ) {
                    onChange({ ...value, [name]: next });
                  }
                }}
                onBlur={(event) => {
                  event.target.value = String(value[name]);
                }}
              />
              <div id={`${id}-description`} className="text-xs text-muted">
                <KeyValueList
                  pairs={[
                    { name: "大きい値", value: settingLabels[name].larger },
                    { name: "範囲", value: `${range.min}–${range.max}` },
                  ]}
                />
              </div>
            </div>
          );
        },
      )}
    </>
  );
}

/** 配置の設定を、button で開く浮いた欄に畳んで出す。図の上を 1 行に保つ。 */
export function LayoutSettingsPopover({
  value,
  onChange,
}: {
  value: CosmosLayoutSettings;
  onChange: (settings: CosmosLayoutSettings) => void;
}) {
  const portalContainer = usePortalContainer();
  return (
    <DialogTrigger>
      <Button variant="ghost">
        <SlidersHorizontal className="size-3.5" aria-hidden="true" />
        配置の設定
      </Button>
      <Popover
        UNSTABLE_portalContainer={portalContainer}
        placement="bottom end"
        className="w-72 rounded-md border border-line bg-surface p-3 shadow-float outline-none"
      >
        <Dialog className="flex flex-col gap-2.5 outline-none">
          <Heading slot="title" className="text-sm font-medium">
            配置の設定
          </Heading>
          <LayoutSettingsInputs value={value} onChange={onChange} />
        </Dialog>
      </Popover>
    </DialogTrigger>
  );
}
