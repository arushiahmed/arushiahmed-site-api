// Package store provides shared S3-backed listing and presigning logic used
// by the photos and documents services.
package store

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const PresignExpiry = 15 * time.Minute

// presignAPI is satisfied by *s3.PresignClient; narrowed to the one method
// Service needs so tests can substitute a fake.
type presignAPI interface {
	PresignGetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
}

type Service struct {
	list    s3.ListObjectsV2APIClient
	presign presignAPI
	bucket  string
}

func New(client *s3.Client, bucket string) *Service {
	return &Service{
		list:    client,
		presign: s3.NewPresignClient(client),
		bucket:  bucket,
	}
}

type Item struct {
	Key          string    `json:"key"`
	URL          string    `json:"url"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"lastModified"`
}

// List pages through the bucket under prefix, presigning and collecting
// every object for which filter returns true (or every object, if filter is nil).
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
			url, err := s.PresignGet(ctx, *obj.Key)
			if err != nil {
				log.Printf("presign %s: %v", *obj.Key, err)
				continue
			}
			items = append(items, Item{
				Key:          *obj.Key,
				URL:          url,
				Size:         aws.ToInt64(obj.Size),
				LastModified: aws.ToTime(obj.LastModified),
			})
		}
	}

	return items, nil
}

// PresignGet returns a presigned GET URL for key, valid for PresignExpiry.
func (s *Service) PresignGet(ctx context.Context, key string) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(PresignExpiry))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}
