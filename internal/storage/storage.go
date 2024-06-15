package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"

	"github.com/aws/aws-sdk-go/aws/request"
	"io"

	"golang.org/x/exp/slog"
)

type S3Client struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	Logger          *slog.Logger
}

func NewS3Client(awsAccessKey, awsSecretKey, awsRegion, bucketName string, logger *slog.Logger) S3Client {
	return S3Client{
		Region:          awsRegion,
		AccessKeyID:     awsAccessKey,
		SecretAccessKey: awsSecretKey,
		Bucket:          bucketName,
		Logger:          logger,
	}
}

func (s *S3Client) RenameFile(originalKey string, newKey string) (string, error) {

	//Initialize a session
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(s.Region),
		Credentials: credentials.NewStaticCredentials(s.AccessKeyID, s.SecretAccessKey, ""),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // 30 seconds timeout
	defer cancel()

	if err != nil {
		return "", err
	}

	fmt.Println("Renaming file")
	svc := s3.New(sess)
	_, err = svc.CopyObject(&s3.CopyObjectInput{
		Bucket:     aws.String(s.Bucket),
		CopySource: aws.String(s.Bucket + "/" + originalKey),
		Key:        aws.String(newKey),
		ACL:        aws.String("public-read"),
	})

	if err != nil {
		s.Logger.Error("failed to copy object file from %s to %s,  %v", originalKey, newKey, err)
		return "", err
	}

	s.Logger.Debug("Waiting for file to be copied", slog.String("originalKey", originalKey), slog.String("newKey", newKey))
	err = svc.WaitUntilObjectExistsWithContext(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(newKey),
	})

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			// Handle timeout scenario
			println("Timeout reached, object still does not exist")
		} else {
			// Handle other errors
			println("Failed to check object existence:", err)
		}
	}
	_, err = svc.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(originalKey),
	})

	if err != nil {
		err = fmt.Errorf("failed to delete object file %s, %v", originalKey, err)
		return "", err
	}

	fmt.Println("Waiting for old file to be deleted")
	err = svc.WaitUntilObjectNotExistsWithContext(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(originalKey),
	})
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			// Handle timeout scenario
			println("Timeout reached, object still exists")
		} else {
			// Handle other errors
			println("Failed to check object existence:", err)
		}
	}
	fmt.Println("File successfully renamed")

	url := fmt.Sprintf("https://%s.s3.amazonaws.com/%s", s.Bucket, newKey)

	return url, nil
}

// bucket, originalKey, newKey, region, accessKeyID, secretAccessKey string

// UploadImageToS3 uploads a file to an S3 bucket.
func (s *S3Client) UploadImageToS3(key string, file io.Reader) (string, error) {
	// Initialize a session in the given region that the SDK will use to load
	// credentials from the shared credentials file ~/.aws/credentials.
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(s.Region),
		Credentials: credentials.NewStaticCredentials(s.AccessKeyID, s.SecretAccessKey, ""),
	})
	if err != nil {
		return "", fmt.Errorf("error creating AWS session: %v", err)
	}

	// Create an uploader with the session and default options
	uploader := s3manager.NewUploader(sess)

	// Upload the file to S3.
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
		Body:   file,
		ACL:    aws.String("public-read"),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload file, %v", err)
	}
	fmt.Printf("Successfully uploaded %q to %q\n", key, s.Bucket)
	url := fmt.Sprintf("https://%s.s3.amazonaws.com/%s", s.Bucket, key)
	return url, nil
}

func waitForObject(svc *s3.S3, bucket, key string) error {
	// Context with timeout, adjust time as needed
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	waiter := request.Waiter{
		Name:        "WaitUntilObjectExists",
		MaxAttempts: 6,                                            // Max number of attempts
		Delay:       request.ConstantWaiterDelay(5 * time.Second), // Delay between each attempt
		Acceptors: []request.WaiterAcceptor{
			{
				State:    request.SuccessWaiterState,
				Matcher:  request.PathAnyWaiterMatch,
				Argument: "StatusCode",
				Expected: 200,
			},
			{
				State:    request.RetryWaiterState,
				Matcher:  request.ErrorWaiterMatch,
				Argument: "",
				Expected: "NotFound",
			},
		},
		NewRequest: func(opts []request.Option) (*request.Request, error) {
			req, _ := svc.HeadObjectRequest(&s3.HeadObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(key),
			})
			req.SetContext(ctx)
			req.ApplyOptions(opts...)
			return req, nil
		},
	}
	return waiter.WaitWithContext(ctx)
}

func waitForObjectDeletion(svc *s3.S3, bucket, key string) error {
	// Context with timeout, adjust time as needed
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	waiter := request.Waiter{
		Name:        "WaitUntilObjectNotExists",
		MaxAttempts: 6,                                            // Adjust based on how long you are willing to wait
		Delay:       request.ConstantWaiterDelay(5 * time.Second), // Delay between attempts
		Acceptors: []request.WaiterAcceptor{
			{
				State:    request.SuccessWaiterState,
				Matcher:  request.PathAnyWaiterMatch,
				Argument: "StatusCode",
				Expected: 404,
			},
			{
				State:    request.FailureWaiterState,
				Matcher:  request.PathAnyWaiterMatch,
				Argument: "StatusCode",
				Expected: 200,
			},
		},
		NewRequest: func(opts []request.Option) (*request.Request, error) {
			req, _ := svc.HeadObjectRequest(&s3.HeadObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(key),
			})
			req.SetContext(ctx)
			req.ApplyOptions(opts...)
			return req, nil
		},
		Logger: aws.NewDefaultLogger(), // Optional: for debugging
	}
	fmt.Println("Waiting for object to be deleted...")
	return waiter.WaitWithContext(ctx)
}
