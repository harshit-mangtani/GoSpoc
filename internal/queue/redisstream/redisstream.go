package redisstream

import (
	"context"
	"strings"

	"github.com/redis/go-redis/v9"
)

type Queue struct {
	client *redis.Client
	stream string
	group string
}

// stream creation 
// XREAD
// XADD


func New(ctx context.Context, client *redis.Client, stream, group string) (*Queue, error) {
	 q:= &Queue{
			client: client,
			stream: stream,
			group: group,
		}

	err:= client.XGroupCreateMkStream(ctx, stream, group, "$").Err()

    if err!=nil && !strings.Contains(err.Error(), "BUSYGROUP")	{
       return nil, err
    }

    return q,nil
}

