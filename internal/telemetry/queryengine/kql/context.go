package kql

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

var ErrBudget = errors.New("KQL query budget exceeded")

type LogContext struct {
	EventID       string
	Before, After int
}

var contextSyntax = regexp.MustCompile(`(?i)^context\s+("(?:[^"\\]|\\.)*")\s+before\s+(\d+)\s+after\s+(\d+)$`)

func ParseContext(stage string) (LogContext, error) {
	var value LogContext
	parts := contextSyntax.FindStringSubmatch(stage)
	if parts == nil {
		return value, fmt.Errorf("expected context event before N after N")
	}
	var err error
	value.EventID, err = strconv.Unquote(parts[1])
	if err != nil || value.EventID == "" || len(value.EventID) > 256 {
		return value, fmt.Errorf("invalid context event id")
	}
	value.Before, err = strconv.Atoi(parts[2])
	if err != nil {
		return value, err
	}
	value.After, err = strconv.Atoi(parts[3])
	if err != nil {
		return value, err
	}
	if value.Before > 200 || value.After > 200 {
		return value, fmt.Errorf("context exceeds neighbor budget")
	}
	return value, nil
}
func FormatContext(value LogContext) string {
	return fmt.Sprintf("context %s before %d after %d", strconv.Quote(value.EventID), value.Before, value.After)
}

func contextQuery(table, selection, where string, args []any, value LogContext) (string, []any) {
	// Context never leaves the authorized window or stream. Canonicalized map
	// entries avoid differences caused only by map serialization order.
	// Rank the joined stream once. ClickHouse inlines CTEs; three UNION branches
	// would reread both the stream and anchor and multiply scan budget usage.
	query := `WITH scoped AS (SELECT * FROM ` + "`" + table + "`" + ` FINAL WHERE ` + where + `),
 anchor AS (SELECT * FROM scoped WHERE event_id=? ORDER BY timestamp,event_id LIMIT 1),
 stream AS (SELECT l.*,a.timestamp AS anchor_time,a.event_id AS anchor_id FROM scoped l INNER JOIN anchor a ON
 l.resource_id=a.resource_id AND l.source_id=a.source_id AND l.service_name=a.service_name AND l.scope_name=a.scope_name
 AND arraySort(arrayZip(mapKeys(l.stream_labels),mapValues(l.stream_labels)))=arraySort(arrayZip(mapKeys(a.stream_labels),mapValues(a.stream_labels)))
 AND l.resource_attributes['service.instance.id']=a.resource_attributes['service.instance.id']
 AND l.resource_attributes['k8s.pod.uid']=a.resource_attributes['k8s.pod.uid']
 AND l.resource_attributes['container.id']=a.resource_attributes['container.id']
 AND l.resource_attributes['deployment.environment.name']=a.resource_attributes['deployment.environment.name']
 AND l.resource_attributes['deployment.environment']=a.resource_attributes['deployment.environment']
 AND l.structured_metadata['log.file.path']=a.structured_metadata['log.file.path'])
 SELECT ` + selection + ` FROM (
 SELECT *, row_number() OVER (PARTITION BY __argus_context_side ORDER BY
 if(__argus_context_side=-1,timestamp,toDateTime64(0,9)) DESC,
 if(__argus_context_side=-1,event_id,'') DESC,timestamp,event_id) AS __argus_context_position
 FROM (SELECT *, multiIf((timestamp,event_id)<(anchor_time,anchor_id),-1,
 (timestamp,event_id)>(anchor_time,anchor_id),1,0) AS __argus_context_side FROM stream)
 ) WHERE (__argus_context_side=-1 AND __argus_context_position<=?)
 OR (__argus_context_side=0 AND __argus_context_position=1)
 OR (__argus_context_side=1 AND __argus_context_position<=?) ORDER BY timestamp,event_id`
	return query, append(args, value.EventID, value.Before, value.After)
}
