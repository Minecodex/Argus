package connector

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func collectorReconciliationFrames(request *connectorv1.CommandReconcileRequest, epoch uint64, records map[string]commandRecord) []*connectorv1.ConnectRequest {
	frames := []*connectorv1.ConnectRequest{}
	if request == nil || request.GetConnectionEpoch() != epoch || len(request.GetCommandIds()) > 128 {
		return frames
	}
	seen := map[string]bool{}
	for _, id := range request.GetCommandIds() {
		if seen[id] {
			continue
		}
		seen[id] = true
		record, ok := records[id]
		if !ok || record.CommandID != id || (record.Status != "succeeded" && record.Status != "failed") || len(record.Result) > 1<<20 {
			continue
		}
		var typed anypb.Any
		var receipt connectorv1.CollectorManagementResult
		if proto.Unmarshal(record.Result, &typed) != nil || !typed.MessageIs(&receipt) || typed.UnmarshalTo(&receipt) != nil {
			continue
		}
		digest := sha256.Sum256(record.Result)
		if !strings.EqualFold(record.ResultHash, hex.EncodeToString(digest[:])) {
			continue
		}
		frames = append(frames, recordedResultFrame(&connectorv1.ConnectorCommand{CommandId: id}, epoch, record))
	}
	return frames
}
