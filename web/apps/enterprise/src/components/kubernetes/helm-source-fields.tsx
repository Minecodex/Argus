import { useState } from "react";
import {
  Button,
  Field,
  Input,
  ProtectedTextarea,
  Select,
  Switch,
} from "@argus/ui";
import { useTranslation } from "react-i18next";
import type { KubernetesHelmPreviewInput } from "@argus/api-client";

export type HelmSourceKind = "upload" | "repository" | "oci";
export type HelmSource = NonNullable<KubernetesHelmPreviewInput["source"]>;
export type HelmSourceCredentials = NonNullable<
  KubernetesHelmPreviewInput["source_credentials"]
>;

export function HelmSourceFields({
  kind,
  source,
  credentials,
  onKind,
  onSource,
  onCredentials,
}: {
  kind: HelmSourceKind;
  source: HelmSource;
  credentials: HelmSourceCredentials;
  onKind: (v: HelmSourceKind) => void;
  onSource: (v: HelmSource) => void;
  onCredentials: (v: HelmSourceCredentials) => void;
}) {
  const { t } = useTranslation();
  const [advanced, setAdvanced] = useState(false);
  return (
    <>
      <Field label={t("kubernetesHelm.sourceKind")} requirement="required">
        <Select
          value={kind}
          options={(["upload", "repository", "oci"] as const).map((value) => ({
            value,
            label: t(`kubernetesHelm.sourceKinds.${value}`),
          }))}
          onValueChange={(v) => onKind(v as HelmSourceKind)}
        />
      </Field>
      {kind !== "upload" && (
        <>
          <Field
            label={t("kubernetesHelm.sourceURL")}
            requirement="required"
            hint={t(`kubernetesHelm.sourceHints.${kind}`)}
          >
            <Input
              value={source.url}
              onChange={(e) => onSource({ ...source, url: e.target.value })}
              required
              maxLength={2048}
              autoComplete="off"
            />
          </Field>
          {kind === "repository" && (
            <Field
              label={t("kubernetesHelm.sourceChart")}
              requirement="required"
            >
              <Input
                value={source.chart ?? ""}
                onChange={(e) => onSource({ ...source, chart: e.target.value })}
                required
                maxLength={128}
                autoComplete="off"
              />
            </Field>
          )}
          <Field
            label={t("kubernetesHelm.sourceVersion")}
            requirement="required"
          >
            <Input
              value={source.version}
              onChange={(e) => onSource({ ...source, version: e.target.value })}
              required
              maxLength={128}
              autoComplete="off"
            />
          </Field>
          <Button
            type="button"
            variant="ghost"
            aria-expanded={advanced}
            onPress={() => setAdvanced((v) => !v)}
          >
            {t("kubernetesHelm.sourceAdvanced")}
          </Button>
          {advanced && (
            <>
              <Field
                label={t("kubernetesHelm.sourceDigest")}
                requirement="optional"
                hint={t("kubernetesHelm.sourceDigestHint")}
              >
                <Input
                  value={source.digest ?? ""}
                  onChange={(e) =>
                    onSource({ ...source, digest: e.target.value })
                  }
                  maxLength={71}
                  autoComplete="off"
                />
              </Field>
              <Field
                label={t("kubernetesHelm.sourceUsername")}
                requirement="optional"
              >
                <Input
                  value={credentials.username ?? ""}
                  onChange={(e) =>
                    onCredentials({ ...credentials, username: e.target.value })
                  }
                  maxLength={256}
                  autoComplete="off"
                />
              </Field>
              <Field
                label={t("kubernetesHelm.sourcePassword")}
                requirement="optional"
              >
                <Input
                  type="password"
                  value={credentials.password ?? ""}
                  onChange={(e) =>
                    onCredentials({ ...credentials, password: e.target.value })
                  }
                  maxLength={4096}
                  autoComplete="new-password"
                />
              </Field>
              <Field
                label={t("kubernetesHelm.sourceToken")}
                requirement="optional"
                hint={t("kubernetesHelm.sourceTokenHint")}
              >
                <Input
                  type="password"
                  value={credentials.token ?? ""}
                  onChange={(e) =>
                    onCredentials({ ...credentials, token: e.target.value })
                  }
                  maxLength={16384}
                  autoComplete="new-password"
                />
              </Field>
              <Field
                label={t("kubernetesHelm.sourceCA")}
                requirement="optional"
              >
                <ProtectedTextarea
                  value={credentials.ca_pem ?? ""}
                  onChange={(e) =>
                    onCredentials({ ...credentials, ca_pem: e.target.value })
                  }
                  rows={3}
                  maxLength={32768}
                  autoComplete="off"
                  spellCheck={false}
                />
              </Field>
              {kind === "oci" && (
                <Field
                  label={t("kubernetesHelm.sourceAuthURL")}
                  requirement="optional"
                  hint={t("kubernetesHelm.sourceAuthHint")}
                >
                  <Input
                    value={source.auth_url ?? ""}
                    onChange={(e) =>
                      onSource({ ...source, auth_url: e.target.value })
                    }
                    maxLength={2048}
                    autoComplete="off"
                  />
                </Field>
              )}
              <Field
                label={t("kubernetesHelm.sourceHTTP")}
                requirement="optional"
              >
                <Switch
                  label={t("kubernetesHelm.sourceHTTP")}
                  checked={source.plain_http ?? false}
                  onChange={(v) => onSource({ ...source, plain_http: v })}
                />
              </Field>
            </>
          )}
        </>
      )}
    </>
  );
}
