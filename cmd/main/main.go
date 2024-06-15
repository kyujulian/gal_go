package main

import (
	"fmt"
	"gal/internal/storage"
	"os"

	"golang.org/x/exp/slog"

	"gal/internal/altgen"
	"gal/internal/server"
	"github.com/joho/godotenv"
)

func main() {

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	logger.Info("Starting application")

	var allowedOrigins = []string{"http://localhost:3000"}

	err := godotenv.Load(".env")

	if err != nil {
		fmt.Println("Error loading .env file: ")
		fmt.Println(err)
	}

	dir, _ := os.Getwd()
	fmt.Println("Current directory:", dir)

	bucketName := os.Getenv("BUCKET_NAME")
	awsRegion := os.Getenv("AWS_REGION")
	awsAccessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	awsSecretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	replicateToken := os.Getenv("REPLICATE_API_TOKEN")
	replicateModelIdentifier := os.Getenv("REPLICATE_MODEL_IDENTIFIER")

	// check if the env variables are set
	if bucketName == "" || awsRegion == "" || awsAccessKey == "" || awsSecretKey == "" || replicateToken == "" || replicateModelIdentifier == "" {
		fmt.Println("Environment Variables not properly set")
		return
	}

	s3Client := storage.NewS3Client(awsAccessKey, awsSecretKey, awsRegion, bucketName, logger)
	rClient := altgen.NewReplicateClient(replicateToken, replicateModelIdentifier, logger)

	server.StartServer(&s3Client, &rClient, allowedOrigins, logger)

}
