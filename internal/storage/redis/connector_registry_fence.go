package redis

import (
	"context"
	"errors"
	"time"

	redislib "github.com/redis/go-redis/v9"
)

// SQL owns the session epoch. Redis must never replace or delete a newer
// routing hint when a heartbeat/close from an older connection arrives late.
var connectorRegistrySetScript = redislib.NewScript(`
local prior=redis.call('GET',KEYS[1])
if prior then
 local ok,entry=pcall(cjson.decode,prior)
 local previous=ok and type(entry)=='table' and tonumber(entry.connection_epoch) or nil
 if previous then
  if previous>tonumber(ARGV[1]) then return 0 end
  if previous==tonumber(ARGV[1]) and entry.gateway_instance_id~=ARGV[2] then return 0 end
 end
end
redis.call('SET',KEYS[1],ARGV[3],'PX',ARGV[4])
return 1
`)

var connectorRegistryDeleteScript = redislib.NewScript(`
local prior=redis.call('GET',KEYS[1])
if not prior then return 0 end
local ok,entry=pcall(cjson.decode,prior)
if not ok or type(entry)~='table' or tonumber(entry.connection_epoch)~=tonumber(ARGV[1]) then return 0 end
return redis.call('DEL',KEYS[1])
`)

func (client *Client) setConnectorRegistry(ctx context.Context, connectorID string, entry ConnectorRegistryEntry, value []byte, ttl time.Duration) error {
	if connectorID == "" || entry.ConnectionEpoch < 1 || entry.GatewayInstanceID == "" || ttl.Milliseconds() < 1 {
		return errors.New("invalid connector registry identity or lifetime")
	}
	return connectorRegistrySetScript.Run(ctx, client.Raw, []string{"argus:connector:registry:" + connectorID}, entry.ConnectionEpoch, entry.GatewayInstanceID, value, ttl.Milliseconds()).Err()
}

func (client *Client) DeleteConnectorRegistryEpoch(ctx context.Context, connectorID string, epoch int64) error {
	if client == nil || client.Raw == nil {
		return nil
	}
	return connectorRegistryDeleteScript.Run(ctx, client.Raw, []string{"argus:connector:registry:" + connectorID}, epoch).Err()
}
