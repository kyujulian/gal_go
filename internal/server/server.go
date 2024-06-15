package server

import (
	"bytes"
	"gal/internal/storage"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"golang.org/x/exp/slog"

	"path/filepath"

	"encoding/csv"
	"fmt"
	"gal/internal/altgen"
	"net/http"
	"strings"
)

func uploadHandler(c echo.Context) error {
	// Logger should never fail
	logger, _ := c.Get("logger").(*slog.Logger)

	s3Client, ok := c.Get("s3Client").(*storage.S3Client)

	if !ok {
		logger.Error("s3Client not found in context")
		logger.Debug("Reference Context: ", slog.String("context", fmt.Sprint(c)))
		return echo.NewHTTPError(http.StatusInternalServerError, "s3Client not found in context")
	}

	rClient, ok := c.Get("replicateClient").(*altgen.ReplicateClient)

	if !ok {
		logger.Error("Replicate Client not found in context")
		logger.Debug("Reference Context: ", slog.String("context", fmt.Sprint(c)))
		return echo.NewHTTPError(http.StatusInternalServerError, "Error with Replicate Client")
	}

	file, err := c.FormFile("file")
	if err != nil {
		logger.Error("File not found", err)
		logger.Debug("Reference Context: ", slog.String("context", fmt.Sprint(c)))
		return echo.NewHTTPError(http.StatusInternalServerError, "File not found")
	}

	name := c.FormValue("name")

	if name == "" {
		logger.Error("Name field not found in the submitted form")
		logger.Debug("Reference Context: ", slog.String("context", fmt.Sprint(c)))
		return echo.NewHTTPError(http.StatusInternalServerError)
	}

	src, err := file.Open()

	if err != nil {
		logger.Error("Error opening file", err)
		logger.Debug("File Info: ",
			slog.String("Filename", file.Filename),
			slog.String("Header", fmt.Sprint(file.Header)))
		return echo.NewHTTPError(http.StatusInternalServerError)
	}

	key := file.Filename

	uploadPath := name + "/" + key

	url, err := s3Client.UploadImageToS3(uploadPath, src)
	if err != nil {
		logger.Error("s3Client.UploadImageToS3 failed with error:", err)
		logger.Debug("File Info: ",
			slog.String("Filename", file.Filename),
			slog.String("Header", fmt.Sprint(file.Header)))

		return echo.NewHTTPError(http.StatusInternalServerError)
	}

	predictionOutput, err := rClient.RequestCaption(url)

	if err != nil {
		logger.Error("rClient.RequestCaption failed with error: ", err)
		return echo.NewHTTPError(http.StatusInternalServerError)
	}

	caption := predictionOutput.Caption

	newFileName := strings.ReplaceAll(predictionOutput.Caption, " ", "-")

	url, err = s3Client.RenameFile(uploadPath, name+"/"+newFileName+".jpg")

	if err != nil {
		logger.Error("s3Client.RenameFile failed with error: ", err)
		return echo.NewHTTPError(http.StatusInternalServerError)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"url":     url,
		"caption": caption,
	})
}

func uploadMultipleFilesHandler(c echo.Context) error {

	logger, _ := c.Get("logger").(*slog.Logger)
	s3Client, ok := c.Get("s3Client").(*storage.S3Client)

	if !ok {
		logger.Error("s3Client not found in context")
		logger.Debug("Reference Context: ", slog.String("context", fmt.Sprint(c)))
		return echo.NewHTTPError(http.StatusInternalServerError, "s3Client not found in context")
	}

	rClient, ok := c.Get("replicateClient").(*altgen.ReplicateClient)

	if !ok {
		logger.Error("Replicate Client not found in context")
		logger.Debug("Reference Context: ", slog.String("context", fmt.Sprint(c)))
		return echo.NewHTTPError(http.StatusInternalServerError, "Error with Replicate Client")
	}

	form, err := c.MultipartForm()
	if err != nil {
		logger.Error("Failed to read multipart form", err)
		logger.Debug("Reference Context: ", slog.String("context", fmt.Sprint(c)))
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to read multipart form")
	}

	files := form.File["files"]

	name := c.FormValue("name")

	if name == "" {
		logger.Error("Name not found in the submitted form")
		logger.Debug("Reference Context: ", slog.String("context", fmt.Sprint(c)))
		return echo.NewHTTPError(http.StatusInternalServerError)
	}

	results := make([]map[string]string, 0)

	//Process each file
	for _, file := range files {

		src, err := file.Open()

		if err != nil {
			logger.Error("Error opening file", err)
			logger.Debug("File Info: ",
				slog.String("Filename", file.Filename),
				slog.String("Header", fmt.Sprint(file.Header)))
			return err
		}

		key := file.Filename
		uploadPath := name + "/" + key

		logger.Info("Uploading file to S3", slog.String("key", key))
		url, err := s3Client.UploadImageToS3(uploadPath, src)

		if err != nil {
			fmt.Println("Failed to upload file to S3")
			src.Close()
			continue
		}
		src.Close()

		logger.Info("Requesting caption from Replicate", slog.String("url", url))
		predictionOutput, err := rClient.RequestCaption(url)
		if err != nil {
			fmt.Println("Failed to get caption from Replicate")
			continue
		}

		caption := predictionOutput.Caption

		fileExt := filepath.Ext(file.Filename)

		newFileName := strings.ReplaceAll(predictionOutput.Caption, " ", "-") + fileExt

		logger.Info("Sending rename file Request to S3", slog.String("old_key", key), slog.String("new_key", newFileName))
		newUrl, err := s3Client.RenameFile(uploadPath, name+"/"+newFileName)

		if err != nil {
			logger.Error("Failed to rename file with error", err)

			logger.Debug("File Info", slog.String("Filename", newFileName), slog.String("uploadPath", uploadPath))
			logger.Info("Continuing to next file")
			continue
		}
		results = append(results, map[string]string{
			"url":     newUrl,
			"caption": caption,
		})
	}

	data, err := generateCsv(results, name, s3Client, c)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to generate CSV")
	}

	return c.JSON(http.StatusOK, data)
}

func generateCsv(results []map[string]string, name string, s3Client *storage.S3Client, c echo.Context) (map[string]interface{}, error) {

	logger, _ := c.Get("logger").(*slog.Logger)
	// Create a buffer to hold the CSV data
	var csvBuffer bytes.Buffer
	writer := csv.NewWriter(&csvBuffer)
	// Write CSV header
	writer.Write([]string{"URL", "Caption"})

	// Collect results and write to CSV buffer
	for _, result := range results {
		writer.Write([]string{result["url"], result["caption"]})
	}
	writer.Flush()

	// empty map
	data := map[string]interface{}{}

	// Check for any error while writing to the CSV
	if err := writer.Error(); err != nil {
		logger.Error("Error writing CSV:", err)
		return data, fmt.Errorf("error generating CSV")
	}

	// Upload the CSV file to S3
	csvReader := bytes.NewReader(csvBuffer.Bytes())
	csvFileName := "results.csv" // or dynamically generate a name based on datetime or other criteria
	csvUploadPath := name + "/" + csvFileName

	csvUrl, err := s3Client.UploadImageToS3(csvUploadPath, csvReader)

	if err != nil {
		logger.Error("Failed writings csv to S3", err)
		return data, fmt.Errorf("failed writing csv to S3")
	}

	// Append the URL of the CSV file to the JSON response
	data = map[string]interface{}{
		"files":   results,
		"csv_url": csvUrl,
	}

	return data, nil

}

func StartServer(s3Client *storage.S3Client, rClient *altgen.ReplicateClient, allowedOrigins []string, logger *slog.Logger) {
	e := echo.New()

	// Set middlewares
	e.Use(S3ClientMiddleware(s3Client))
	e.Use(ReplicateClientMiddleware(rClient))
	e.Use(LoggerMiddleware(logger))

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: allowedOrigins, // Allow only specific origin
		AllowMethods: []string{echo.GET, echo.PUT, echo.POST, echo.DELETE},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
	}))

	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello, World!")
	})

	e.POST("/upload", uploadHandler)

	e.POST("/upload_multiple", uploadMultipleFilesHandler)

	e.Logger.Fatal(e.Start(":1323"))

}
