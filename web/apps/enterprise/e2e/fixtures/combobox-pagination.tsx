import { useState } from "react";
import { createRoot } from "react-dom/client";
import { Button, ComboBox, Field, LocaleProvider } from "@argus/ui";
import "@argus/ui/styles.css";

const params = new URLSearchParams(location.search);
const locale = params.get("locale") === "en-US" ? "en-US" : "zh-CN";
document.documentElement.dataset.theme = params.get("theme") ?? "light";

function Fixture() {
  const [fixed, setFixed] = useState("");
  const [value, setValue] = useState("selected"),
    [scope, setScope] = useState("a");
  return (
    <LocaleProvider locale={locale} persist={false}>
      <main
        className="argus-page"
        style={{
          maxWidth: "var(--dialog-width-md)",
          padding: "var(--space-6)",
        }}
      >
        <Button onPress={() => setScope(scope === "a" ? "b" : "a")}>
          Change scope
        </Button>
        <Field label="Metric" requirement="optional">
          <ComboBox
            allowCustom={params.get("custom") === "1"}
            value={value}
            onValueChange={setValue}
            options={[]}
            contextKey={scope}
            fetchOptions={async (search, cursor, signal) => {
              const response = await fetch(
                `/pagination-fixture?${new URLSearchParams({ search, cursor: cursor ?? "", scope })}`,
                { signal },
              );
              if (!response.ok) throw new Error("Candidates unavailable");
              return response.json();
            }}
          />
        </Field>
        <output aria-label="Selected metric">{value}</output>
        <Field label="Static" requirement="optional">
          <ComboBox
            value={fixed}
            onValueChange={setFixed}
            options={[
              { value: "cat", label: "Cat" },
              { value: "dog", label: "Dog" },
            ]}
          />
        </Field>
      </main>
    </LocaleProvider>
  );
}
createRoot(document.getElementById("root")!).render(<Fixture />);
