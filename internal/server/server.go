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

	s3Client, ok := c.Get("s3Client").(*storage.S3Client)

	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "s3Client not found in context")
	}

	rClient, ok := c.Get("replicateClient").(*altgen.ReplicateClient)

	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "replicateClient not found in context")
	}

	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "File not found")
	}

	name := c.FormValue("name")

	if name == "" {
		return echo.NewHTTPError(http.StatusInternalServerError, "Name not found")
	}

	src, err := file.Open()

	if err != nil {
		fmt.Println("Error opening file")
		return err
	}

	key := file.Filename

	uploadPath := name + "/" + key

	url, err := s3Client.UploadImageToS3(uploadPath, src)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Fail to Upload file to S3")
	}

	predictionOutput, err := rClient.RequestCaption(url)

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Fail to get caption from Replicate")
	}

	caption := predictionOutput.Caption

	newFileName := strings.ReplaceAll(predictionOutput.Caption, " ", "-")

	url, err = s3Client.RenameFile(uploadPath, name+"/"+newFileName+".jpg")

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Fail to Upload file to S3")
	}

	return c.JSON(http.StatusOK, map[string]string{
		"url":     url,
		"caption": caption,
	})
}

func uploadMultipleFilesHandler(c echo.Context) error {

	s3Client, ok := c.Get("s3Client").(*storage.S3Client)

	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "s3Client not found in context")
	}

	rClient, ok := c.Get("replicateClient").(*altgen.ReplicateClient)

	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "replicateClient not found in context")
	}

	form, err := c.MultipartForm()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to read multipart form")
	}

	files := form.File["files"]

	name := c.FormValue("name")

	if name == "" {
		return echo.NewHTTPError(http.StatusInternalServerError, "Name not found")
	}

	results := make([]map[string]string, 0)

	//Process each file
	for _, file := range files {
		src, err := file.Open()

		if err != nil {
			fmt.Println("Error opening file")
			return err
		}

		key := file.Filename
		uploadPath := name + "/" + key

		url, err := s3Client.UploadImageToS3(uploadPath, src)

		if err != nil {
			fmt.Println("Failed to upload file to S3")
			src.Close()
			continue
		}
		src.Close()

		predictionOutput, err := rClient.RequestCaption(url)
		if err != nil {
			fmt.Println("Failed to get caption from Replicate")
			continue
		}

		caption := predictionOutput.Caption

		fileExt := filepath.Ext(file.Filename)

		newFileName := strings.ReplaceAll(predictionOutput.Caption, " ", "-") + fileExt
		newUrl, err := s3Client.RenameFile(uploadPath, name+"/"+newFileName)

		if err != nil {
			fmt.Println("Failed to rename file")
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
		fmt.Println("Error writing CSV:", err)
		return data, fmt.Errorf("error generating CSV")
	}

	// Upload the CSV file to S3
	csvReader := bytes.NewReader(csvBuffer.Bytes())
	csvFileName := "results.csv" // or dynamically generate a name based on datetime or other criteria
	csvUploadPath := name + "/" + csvFileName

	csvUrl, err := s3Client.UploadImageToS3(csvUploadPath, csvReader)

	if err != nil {
		fmt.Println("Failed writings csv to S3", err)
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
