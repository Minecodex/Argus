import { useState } from "react";
import { Plus } from "lucide-react";
import { Button, type ControlSize } from "./button";
import { Input, Field, Switch } from "./form";
import { Select } from "./select";
import { ComboBox } from "./combobox";
import { Checkbox } from "./checkbox";
import { RadioOptions } from "./radio-options";
import { DateTimePicker } from "./date-time-picker";
import { ValueSelector } from "./value-selector";
import { useUiText } from "./locale";

/** Development gallery uses the same public controls as the portals. */
export function ControlShowcase() {
  const text = useUiText();
  const [environment, setEnvironment] = useState("production");
  const [search, setSearch] = useState("production");
  const [checked, setChecked] = useState(true);
  const [date, setDate] = useState("2026-10-07T09:00");
  const [resources, setResources] = useState({
    all: true,
    values: [] as string[],
  });
  const options = [
    {
      value: "production",
      label: text("生产环境", "Production"),
      description: text("关键业务", "Critical workloads"),
    },
    { value: "staging", label: text("预发布环境", "Staging") },
    {
      value: "retired",
      label: text("已停用的环境", "Retired environment"),
      disabled: true,
    },
  ];
  return (
    <div className="argus-control-showcase">
      {(["sm", "md", "lg"] as ControlSize[]).map((size) => (
        <div
          className="argus-control-showcase__row"
          data-control-size={size}
          key={size}
        >
          <span>
            {text(
              `${size} · ${size === "sm" ? 28 : size === "md" ? 32 : 36} 像素`,
              `${size} · ${size === "sm" ? 28 : size === "md" ? 32 : 36}px`,
            )}
          </span>
          <Button size={size}>{text("操作", "Action")}</Button>
          <Button
            size={size}
            isIconOnly
            aria-label={text(`添加 ${size}`, `Add ${size}`)}
          >
            <Plus />
          </Button>
          <Input
            size={size}
            aria-label={text(`输入 ${size}`, `Input ${size}`)}
            placeholder={text("名称", "Name")}
          />
          <Select
            size={size}
            ariaLabel={text(`环境 ${size}`, `Environment ${size}`)}
            options={options}
            value={environment}
            onValueChange={setEnvironment}
          />
        </div>
      ))}
      <div className="argus-control-showcase__row">
        <Button isPending>{text("处理中", "Working")}</Button>
        <Button isDisabled>{text("已禁用", "Disabled")}</Button>
        <Checkbox isSelected={checked} onChange={setChecked}>
          {text("选择资源", "Select resource")}
        </Checkbox>
        <Checkbox isDisabled>{text("无操作权限", "Not editable")}</Checkbox>
        <Switch
          checked={checked}
          onChange={setChecked}
          label={text("开启", "Enable")}
        />
      </div>
      <div className="argus-control-showcase__fields">
        <Field
          requirement="required"
          label={text("可搜索环境", "Search environment")}
        >
          <ComboBox
            options={options}
            value={search}
            onValueChange={setSearch}
          />
        </Field>
        <Field
          requirement="required"
          label={text("有错误的名称", "Invalid name")}
          error={text("名称不能为空", "Name is required")}
        >
          <Input />
        </Field>
        <Field
          requirement="optional"
          label={text("日期和时间", "Date and time")}
        >
          <DateTimePicker value={date} onChange={setDate} />
        </Field>
        <ValueSelector
          label={text("资源多选", "Select resources")}
          value={resources}
          onChange={setResources}
          candidates={["host-a", "host-b", "host-c"]}
          multiple
        />
      </div>
      <RadioOptions
        label={text("环境单选", "Choose environment")}
        options={options}
        value={environment}
        onChange={setEnvironment}
      />
    </div>
  );
}
