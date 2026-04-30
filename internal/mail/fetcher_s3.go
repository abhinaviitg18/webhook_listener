package mail

import (
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Fetcher struct {
	Client *s3.Client
}

func (f S3Fetcher) Fetch(ctx context.Context, bucket, key string) ([]byte, error) {
	resp, err := f.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
