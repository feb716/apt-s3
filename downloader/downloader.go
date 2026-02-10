// Package downloader parses an s3 URI and downloads the specified file to the
// filesystem.
package downloader

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// Downloader tracks the region and AWS config and only recreates the config
// if the region has changed
type Downloader struct {
	region string
	cfg    aws.Config
	ctx    context.Context
}

func New(ctx context.Context) *Downloader {
	d := &Downloader{
		ctx: ctx,
	}
	return d
}

// Use default AWS credential chain
func (d *Downloader) loadCredentials(region string) (aws.Config, error) {
	cfg, err := config.LoadDefaultConfig(d.ctx, config.WithRegion(region))
	return cfg, err
}

// parseUri takes an S3 URI s3://<bucket>.s3.<region>.amazonaws.com/key/file
// and returns the bucket, region, key, and filename
func (d *Downloader) parseURI(keyString string) (string, string, string, string) {
	ss := strings.Split(keyString, "/")
	bucketSs := strings.Split(ss[2], ".")
	bucket := bucketSs[0]
	region := bucketSs[2]
	// Default to us-east-1 if just <bucket>.s3.amazonaws.com is passed
	if region == "amazonaws" {
		region = "us-east-1"
	}
	key := strings.Join(ss[3:], "/")
	filename := ss[len(ss)-1]
	return bucket, region, key, filename
}

// GetFileAttributes queries the object in S3 and returns the timestamp and
// size in the format expected by apt
func (d *Downloader) GetFileAttributes(s3Uri string) (string, int64, error) {
	var err error
	bucket, region, key, _ := d.parseURI(s3Uri)

	if d.region != region {
		d.region = region
		d.cfg, err = d.loadCredentials(region)
		if err != nil {
			return "", -1, err
		}
	}

	client := s3.NewFromConfig(d.cfg)

	result, err := client.GetObject(d.ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) {
			return "", -1, errors.New(strings.Join(strings.Split(ae.Error(), "\n"), " "))
		}
		return "", -1, err
	}

	return result.LastModified.Format("2006-01-02T15:04:05+00:00"), *result.ContentLength, nil
}

// DownloadFile pulls the file from an S3 bucket and writes it to the specified
// path
func (d *Downloader) DownloadFile(s3Uri string, path string) (string, error) {
	var err error
	bucket, region, key, filename := d.parseURI(s3Uri)
	if path != "" {
		filename = path
	}

	if d.region != region {
		d.region = region
		d.cfg, err = d.loadCredentials(region)
		if err != nil {
			return "", err
		}
	}

	client := s3.NewFromConfig(d.cfg)
	downloader := manager.NewDownloader(client)

	f, err := os.Create(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := downloader.Download(d.ctx, f, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}); err != nil {
		os.Remove(filename)
		return "", err
	}
	return filename, nil
}
