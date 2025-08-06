package main

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func GetObjects() ([]types.Object, error) {
	var outputs []types.Object
	var continuationToken *string
	for {
		output, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(config.AWSBucket),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, err
		}

		outputs = append(outputs, output.Contents...)

		// If more objects to retrieve, continue
		if *output.IsTruncated {
			continuationToken = output.NextContinuationToken
		} else {
			break
		}
	}

	return outputs, nil
}

func DownloadObject(obj types.Object) (*s3.GetObjectOutput, error) {
	getOutput, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(config.AWSBucket),
		Key:    aws.String(*obj.Key),
	})
	if err != nil {
		return nil, err
	}

	return getOutput, nil
}

func DeleteObject(obj types.Object) error {
	_, err := s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(config.AWSBucket),
		Key:    aws.String(*obj.Key),
	})
	return err
}
