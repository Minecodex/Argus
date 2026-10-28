import schema from "../../../../internal/telemetry/queryengine/skywalking/schema/trace.graphql?raw";
// Compile the same read-only SDL as the server, without production introspection.
export const observationTraceSchema = schema;
