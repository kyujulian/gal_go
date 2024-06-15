package server

import (
	"gal/internal/altgen"
	"gal/internal/storage"

	"github.com/labstack/echo/v4"

	"golang.org/x/exp/slog"
)

// Middlewares
func S3ClientMiddleware(s3Client *storage.S3Client) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("s3Client", s3Client)
			return next(c)
		}
	}
}

func LoggerMiddleware(logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("logger", logger)
			return next(c)
		}
	}
}

func ReplicateClientMiddleware(replicateClient *altgen.ReplicateClient) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("replicateClient", replicateClient)
			return next(c)
		}
	}
}
