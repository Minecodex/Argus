"""Static human report text; source values and machine JSON stay unchanged."""

LOCALES = ("en-US", "zh-CN")

ZH = {
    "Evaluation time comparison": "评估时间比较",
    "Time fact": "时间事实", "Computed result": "计算结果",
    "Requested duration (seconds)": "请求窗口时长（秒）",
    "Returned evaluation span (seconds)": "返回评估首末点的时间跨度（秒）",
    "First evaluation relative to requested start": "首个评估点相对请求起点",
    "First evaluation minus requested start (seconds)": "首个评估点减请求起点（秒）",
    "Last evaluation relative to requested end": "末个评估点相对请求终点",
    "Last evaluation minus requested end (seconds)": "末个评估点减请求终点（秒）",
    "after": "晚于", "before": "早于", "equal": "相同",
    "Offsets are signed: positive means after, negative means before. The span is elapsed time between returned endpoints; it does not prove continuous raw sampling or ingestion completeness.": "时间差有正负：正数表示晚于，负数表示早于。跨度仅是返回首末点评估时间之差，不证明原始采样连续或摄取完整。",
    "Metric evidence": "指标证据",
    "Statistics cover the returned observations. Endpoint values retain their corresponding UTC timestamps; extrema describe the value range.": "统计仅覆盖返回的观测。端点值保留对应 UTC 时间；最小值和最大值描述值域。",
    "Metric": "指标", "Requested UTC window": "请求的 UTC 窗口", "to": "至",
    "Returned observations": "返回观测数", "UTC range": "UTC 观测范围",
    "Source unit": "源单位", "display unit": "显示单位", "conversion": "换算",
    "Metric type": "指标类型",
    "These tables describe cumulative readings. Interval volume, throughput and activity direction require reset-aware queries; counter magnitude alone cannot establish them. No such computation was performed here.": "下表描述累计读数。区间流量、吞吐与活动方向需要考虑计数器重置的查询；仅凭累计值大小无法确定。本报告未进行该计算。",
    "Metric type is unknown in the original query evidence; a caller hint does not certify it. Verify type before interpreting interval activity or rates.": "原查询证据中的指标类型未知，调用方提示不能认证类型。解释区间活动或速率前需先验证类型。",
    "Series": "序列", "Points": "点数", "Raw minimum": "原始最小值", "Raw maximum": "原始最大值",
    "Raw mean": "原始均值", "Display minimum": "显示最小值", "Display maximum": "显示最大值", "Display mean": "显示均值",
    "First UTC": "首点 UTC", "First raw": "首点原值", "First display": "首点显示值",
    "Last UTC": "末点 UTC", "Last raw": "末点原值", "Last display": "末点显示值",
    "Dashboard panel evidence": "仪表盘面板证据",
    "Statistics cover returned values from the published queries; units apply to those expression outputs.": "统计覆盖已发布查询的返回值；单位对应这些表达式的输出。",
    "Panel / target": "面板 / 目标", "State": "状态", "Minimum": "最小值", "Maximum": "最大值", "Output unit": "输出单位",
    "numeric": "数值", "no_data": "无数据", "unavailable": "不可用",
    "Interpretation": "分析说明", "Evidence summary": "完整证据",
    "First value": "首点值", "Last value": "末点值",
    "Endpoint values retain their actual timestamps; a minimum or maximum is not automatically an endpoint.": "首末点值对应其实际时间戳；最小值或最大值不自动等于首末点值。",
    "Interval activity level and steadiness remain unknown from this input. Increasing cumulative readings alone do not establish quiet, busy or steady network activity; separate reset-aware evidence is needed.": "仅凭本输入，区间活动强弱与稳定性仍未知。累计读数递增本身不能证明网络活动低、高或平稳，仍需独立的重置感知证据。",
    "Manifest checksum verification": "清单期望校验值核对",
    "not_requested": "未请求", "verified": "已验证", "unknown": "未知",
    "Matched the supplied expected manifest SHA256.": "已与提供的清单期望 SHA256 核对一致。",
    "No expected manifest SHA256 was supplied. This is not a checksum mismatch; panel files were separately checked against their sealed hashes.": "未提供清单的期望 SHA256。这不表示校验值不一致；各面板原件已分别与其封存 Hash 核对。",
}


def translator(locale):
    if locale not in LOCALES:
        raise ValueError("report locale must be en-US or zh-CN")
    return lambda value: ZH.get(value, value) if locale == "zh-CN" else value


def table_header(t, values):
    return "| " + " | ".join(t(value) for value in values) + " |\n" + "| " + " | ".join("---" for _ in values) + " |\n"
