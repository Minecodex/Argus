import {
  useEffect,
  useMemo,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { FormProvider, useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { useMutation, useQuery } from "@tanstack/react-query";
import { LoaderCircle } from "lucide-react";
import {
  ApiError,
  apiErrorRequestId,
  passwordPolicyRuleFromError,
  useApi,
  type SetupSubmission,
} from "@argus/api-client";
import {
  Alert,
  AppearanceControls,
  Badge,
  Button,
  Card,
  CardContent,
} from "@argus/ui";
import { SetupFields } from "./setup-fields";
import { setupCredentialStore } from "./setup-credential";
import {
  createInitialDraft,
  createSetupSchemas,
  toSubmission,
  type SetupDraft,
} from "./validation";

/**
 * 一次性系统初始化向导（docs/07 §2-4）。
 * 仅平台 `uninitialized` 状态可进入；提交为单事务，成功后向导永久关闭。
 */
export function PlatformSetupGate({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const api = useApi();
  const [initialized, setInitialized] = useState(false);
  const setupToken = useSyncExternalStore(
    setupCredentialStore.subscribe,
    setupCredentialStore.getSnapshot,
    setupCredentialStore.getSnapshot,
  );

  const statusQuery = useQuery({
    queryKey: ["setup", "status"],
    queryFn: () => api.setup.status(),
    enabled: !initialized,
    retry: 1,
    refetchOnWindowFocus: false,
  });
  const platformState = statusQuery.data?.state;

  useEffect(() => {
    // Login clears the query cache. Keep the one-way setup decision in the gate
    // so that cache invalidation cannot unmount the login router.
    if (platformState === "initialized") setInitialized(true);
  }, [platformState]);

  const schemas = useMemo(() => createSetupSchemas(t), [t]);
  const form = useForm<SetupDraft>({
    resolver: zodResolver(schemas.setup),
    defaultValues: createInitialDraft(setupToken),
    mode: "onTouched",
  });

  useEffect(() => {
    if (!setupToken || form.getValues("setupToken") === setupToken) return;
    form.setValue("setupToken", setupToken, { shouldValidate: true });
  }, [form, setupToken]);

  const submitMutation = useMutation({
    mutationFn: (input: SetupSubmission) => api.setup.submit(input),
    gcTime: 0,
    onSuccess: () => {
      submitMutation.reset();
      form.reset(createInitialDraft(""));
      setupCredentialStore.clear();
      window.history.replaceState(null, "", "/login");
      setInitialized(true);
    },
  });

  const submitErrorDescription = (() => {
    if (!(submitMutation.error instanceof ApiError)) {
      return t("setup.submit.genericError");
    }
    let description: string;
    if (
      ["SETUP_TOKEN_INVALID", "SETUP_TOKEN_EXPIRED"].includes(
        submitMutation.error.code,
      )
    ) {
      description = t("setup.submit.tokenInvalid");
    } else {
      const passwordRule = passwordPolicyRuleFromError(submitMutation.error);
      description = passwordRule
        ? t(`setup.system.password.${passwordRule}`)
        : t("setup.submit.genericError");
    }
    const requestId = apiErrorRequestId(submitMutation.error);
    return requestId
      ? `${description} ${t("setup.submit.requestReference", { requestId })}`
      : description;
  })();

  if (initialized || platformState === "initialized") {
    return <InitializedPlatformEntry>{children}</InitializedPlatformEntry>;
  }

  let content: ReactNode;
  if (statusQuery.isPending) {
    content = (
      <Card className="argus-setup-card">
        <CardContent>
          <div className="argus-setup-loading">
            <LoaderCircle aria-hidden className="argus-spin" size={20} />
            <span>{t("setup.loading")}</span>
          </div>
        </CardContent>
      </Card>
    );
  } else if (statusQuery.isError) {
    content = (
      <Card className="argus-setup-card">
        <CardContent>
          <div className="argus-setup-fields">
            <Alert
              description={t("setup.statusError.description")}
              title={t("setup.statusError.title")}
              tone="danger"
            />
            <Button onPress={() => statusQuery.refetch()} variant="secondary">
              {t("setup.statusError.retry")}
            </Button>
          </div>
        </CardContent>
      </Card>
    );
  } else if (statusQuery.data.state !== "uninitialized") {
    content = (
      <Card className="argus-setup-card">
        <CardContent>
          <div className="argus-setup-loading">
            <LoaderCircle aria-hidden className="argus-spin" size={20} />
            <span>{t("setup.initializing")}</span>
          </div>
        </CardContent>
      </Card>
    );
  } else if (!schemas.credential.safeParse({ setupToken }).success) {
    content = (
      <Card className="argus-setup-card">
        <CardContent>
          <Alert
            description={t("setup.credentialMissing.description")}
            title={t("setup.credentialMissing.title")}
            tone="danger"
          />
        </CardContent>
      </Card>
    );
  } else {
    content = (
      <Card className="argus-setup-card">
        <CardContent>
          <FormProvider {...form}>
            <form
              className="argus-setup-fields"
              onSubmit={form.handleSubmit((values) =>
                submitMutation.mutate(toSubmission(values)),
              )}
            >
              <SetupFields />
              {submitMutation.isError && (
                <Alert
                  description={submitErrorDescription}
                  title={t("setup.submit.errorTitle")}
                  tone="danger"
                />
              )}
              <Button type="submit" isPending={submitMutation.isPending}>
                {t("setup.submit.label")}
              </Button>
            </form>
          </FormProvider>
        </CardContent>
      </Card>
    );
  }

  return (
    <main className="argus-setup-shell">
      <div className="argus-setup-appearance">
        <AppearanceControls />
      </div>
      <header className="argus-setup-header">
        <div className="argus-setup-brand">◉</div>
        <Badge tone="accent">{t("setup.badge")}</Badge>
        <h1>{t("setup.title")}</h1>
        <p>{t("setup.subtitle")}</p>
      </header>
      {content}
    </main>
  );
}

function InitializedPlatformEntry({ children }: { children: ReactNode }) {
  // AuthProvider must restore the persisted session before the router decides
  // whether the current URL needs a login redirect. Rewriting the URL here
  // would race that restore on a hard refresh and strand authenticated users
  // on /login.
  return <>{children}</>;
}
