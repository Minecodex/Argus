import { useUiText } from "./locale";

/** Evaluation timestamps do not establish sample freshness or ingestion completeness. */
export function ObservationFreshnessContent({
  targets,
}: {
  targets: { id: string; meta?: Record<string, unknown> }[];
}) {
  const text = useUiText();
  const timestamp = (value: unknown) =>
    typeof value === "string" && Number.isFinite(Date.parse(value)) ? (
      <time dateTime={value}>{new Date(value).toISOString()}</time>
    ) : (
      text("未知", "Unknown")
    );
  return (
    <section className="argus-observation-freshness">
      {targets.map(({ id, meta }) => (
        <div key={id}>
          <strong>{id}</strong>
          {" · "}
          {text("查询完成", "Query completed")}:{" "}
          {timestamp(meta?.query_completed_at)}
          {meta?.cache_hit === true &&
            ` · ${text("缓存结果", "Cached result")}`}
          <br />
          {text(
            "已读取样本的最新事件时间",
            "Latest event among observed query samples",
          )}
          :{" "}
          {timestamp(
            meta?.sample_time_basis === "observed_query_samples"
              ? meta.latest_sample_at
              : undefined,
          )}
          {" · "}
          {text("摄入完整性", "Ingestion completeness")}:{" "}
          {text("未知", "Unknown")}
        </div>
      ))}
      <p>
        {text(
          "样本时间仅说明本次查询观察到的数据，不证明整个时段已完整到达。",
          "Sample time describes data observed by this query; it does not establish complete arrival for the interval.",
        )}
      </p>
    </section>
  );
}
