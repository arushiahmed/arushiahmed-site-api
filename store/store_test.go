package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// fakeLister serves ListObjectsV2 from a fixed in-memory page, ignoring
// pagination tokens since tests only need a single page.
type fakeLister struct {
	objects []types.Object
	err     error
}

func (f *fakeLister) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &s3.ListObjectsV2Output{Contents: f.objects}, nil
}

func TestList(t *testing.T) {
	now := time.Now()
	lister := &fakeLister{objects: []types.Object{
		{Key: aws.String("paris/eiffel-tower.jpg"), Size: aws.Int64(100), LastModified: aws.Time(now)},
		{Key: aws.String("tokyo/shibuya.jpg"), Size: aws.Int64(200), LastModified: aws.Time(now)},
		{Key: aws.String("folder/"), Size: aws.Int64(0), LastModified: aws.Time(now)}, // directory marker, should be skipped
	}}
	svc := &Service{list: lister, bucket: "test-bucket", cdnDomain: "cdn.example.com"}

	items, err := svc.List(context.Background(), "", nil)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items (directory marker skipped), got %d: %+v", len(items), items)
	}
	if items[0].Key != "paris/eiffel-tower.jpg" || items[0].URL != "https://cdn.example.com/paris/eiffel-tower.jpg" {
		t.Errorf("unexpected first item: %+v", items[0])
	}
	if items[0].Size != 100 || !items[0].LastModified.Equal(now) {
		t.Errorf("size/lastModified not carried through: %+v", items[0])
	}
}

func TestList_Filter(t *testing.T) {
	lister := &fakeLister{objects: []types.Object{
		{Key: aws.String("paris/eiffel-tower.jpg"), Size: aws.Int64(1), LastModified: aws.Time(time.Now())},
		{Key: aws.String("tokyo/shibuya.jpg"), Size: aws.Int64(1), LastModified: aws.Time(time.Now())},
	}}
	svc := &Service{list: lister, bucket: "test-bucket", cdnDomain: "cdn.example.com"}

	items, err := svc.List(context.Background(), "", func(key string) bool {
		return strings.Contains(strings.ToLower(key), "paris")
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 1 || items[0].Key != "paris/eiffel-tower.jpg" {
		t.Fatalf("expected only the paris item, got %+v", items)
	}
}

func TestList_PropagatesListError(t *testing.T) {
	svc := &Service{list: &fakeLister{err: errors.New("boom")}, bucket: "test-bucket", cdnDomain: "cdn.example.com"}

	if _, err := svc.List(context.Background(), "", nil); err == nil {
		t.Fatal("expected error from List when the underlying S3 call fails")
	}
}

func TestPublicURL(t *testing.T) {
	svc := &Service{list: &fakeLister{}, bucket: "test-bucket", cdnDomain: "cdn.example.com"}

	url := svc.PublicURL("some/key.jpg")
	if url != "https://cdn.example.com/some/key.jpg" {
		t.Errorf("unexpected URL: %q", url)
	}
}
