package telemetry

// Shared discovery/schema/error guidance names actual supported predicates.
// It does not change the parser, data scope, execution budget or permissions.
const modelLogSyntaxGuidance = `正文包含使用 body : "text" | limit 100，例如 body : "request-id" | limit 100。文本值使用双引号；不要使用 body contains 或 parse body as alias。For a body substring use body : "text" | limit 100. Quote text values with double quotes. body contains and parse body as alias are not valid Argus log DSL.`

const modelOriginalAnalysisGuidance = ` Native results are sealed and delivered to the source-bound Workspace path. A framework-certified complete small result may be analyzed directly; large, clipped or uncertain results require the actual original file. 原件仍封存；框架确认完整的小结果可直接分析，大结果/截断/完整性未知须读取原件。`
