// Package store provides shared S3-backed listing logic, and builds CDN
// URLs for objects, used by the photos and documents services.
package store

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Service struct {
	list      s3.ListObjectsV2APIClient
	bucket    string
	cdnDomain string
}

func New(client *s3.Client, bucket, cdnDomain string) *Service {
	return &Service{
		list:      client,
		bucket:    bucket,
		cdnDomain: cdnDomain,
	}
}

type Item struct {
	Key          string    `json:"key"`
	URL          string    `json:"url"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"lastModified"`
}

// List pages through the bucket under prefix, collecting every object for
// which filter returns true (or every object, if filter is nil).
func (s *Service) List(ctx context.Context, prefix string, filter func(key string) bool) ([]Item, error) {
	items := []Item{}
	paginator := s3.NewListObjectsV2Paginator(s.list, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			if obj.Key == nil || strings.HasSuffix(*obj.Key, "/") {
				continue
			}
			if filter != nil && !filter(*obj.Key) {
				continue
			}
			items = append(items, Item{
				Key:          *obj.Key,
				URL:          s.PublicURL(*obj.Key),
				Size:         aws.ToInt64(obj.Size),
				LastModified: aws.ToTime(obj.LastModified),
			})
		}
	}

	return items, nil
}

// PublicURL returns the CDN URL through which key is served.
func (s *Service) PublicURL(key string) string {
	return "https://" + s.cdnDomain + "/" + key
}
