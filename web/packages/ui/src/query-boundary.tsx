import type { ReactNode } from "react";
import { Alert } from "./code";
import { Button } from "./button";
import { Spinner } from "./display";
import { useUiText } from "./locale";

export type QueryFeedbackState = {
  isLoading?: boolean;
  isPending?: boolean;
  error: unknown;
  refetch: () => unknown;
};

/** A query failure must never masquerade as an empty or healthy list. */
export function QueryBoundary({
  query,
  dependencies = [],
  children,
}: {
  query: QueryFeedbackState;
  dependencies?: readonly QueryFeedbackState[];
  children: ReactNode;
}) {
  const text = useUiText();
  const queries = [query, ...dependencies];
  const isDenied = (error: unknown) =>
    !!error &&
    typeof error === "object" &&
    "status" in error &&
    error.status === 403;
  const failed =
    queries.find((item) => isDenied(item.error)) ??
    queries.find((item) => item.error);
  if (failed) {
    const denied = isDenied(failed.error);
    const missing =
      !!failed.error &&
      typeof failed.error === "object" &&
      "status" in failed.error &&
      failed.error.status === 404;
    const unavailable =
      !!failed.error &&
      typeof failed.error === "object" &&
      "code" in failed.error &&
      failed.error.code === "CLIENT_OPERATION_UNAVAILABLE";
    return (
      <div className="argus-query-feedback" role="alert">
        <Alert
          tone={denied || missing || unavailable ? "warning" : "danger"}
          title={
            denied
              ? text("无权限访问", "Access denied")
              : missing
                ? text("内容不存在", "Content not found")
                : unavailable
                  ? text("当前服务不支持此功能", "This feature is unavailable")
                  : text("数据加载失败", "Failed to load data")
          }
          description={
            denied
              ? text(
                  "当前账号无法访问这些内容，请核对权限后重试。",
                  "This account cannot access this content. Check permissions and retry.",
                )
              : missing
                ? text(
                    "请求的内容不存在或已不可用，请返回列表核对。",
                    "The requested content is missing or unavailable. Check the list and try again.",
                  )
                : unavailable
                  ? text(
                      "服务尚未提供此能力，无法读取或修改相关配置。",
                      "The service does not provide this capability. This configuration cannot be viewed or changed.",
                    )
                  : text(
                      "未能获取数据，不能将此状态视为没有数据。请重试。",
                      "Data could not be retrieved. This is not an empty result. Retry the request.",
                    )
          }
        />
        {!unavailable && (
          <Button
            onPress={() => {
              void Promise.allSettled(
                queries.map(async (item) => item.refetch()),
              );
            }}
          >
            {text("重试", "Retry")}
          </Button>
        )}
      </div>
    );
  }
  if (queries.some((item) => item.isLoading ?? item.isPending))
    return <Spinner label={text("正在加载", "Loading")} />;
  return children;
}
