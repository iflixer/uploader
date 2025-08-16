package storage

import (
	"context"

	"github.com/minio/minio-go/v7"
)

func (c *Client) Stat(ctx context.Context, bucket, remoteKey string) (objectSize int64, err error) {
	stat, err := c.cli.StatObject(ctx, bucket, remoteKey, minio.StatObjectOptions{})
	if err != nil {
		return
	}
	objectSize = stat.Size
	return
}
