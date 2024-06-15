package altgen

import (
	"context"
	"fmt"
	"github.com/replicate/replicate-go"
	"strings"

	"golang.org/x/exp/slog"
)

type ImageOutput struct {
	ImageUrl string `json:"image_url"`
	Caption  string `json:"caption"`
}

type ReplicateClient struct {
	Token           string
	ModelIdentifier string
	Logger          *slog.Logger
}

func NewReplicateClient(token string, modelIdentifier string) ReplicateClient {
	return ReplicateClient{
		Token:           token,
		ModelIdentifier: modelIdentifier,
	}
}

func (r *ReplicateClient) RequestCaption(imageUrl string) (ImageOutput, error) {
	ctx := context.Background()

	// You can also provide a token directly with
	r8, err := replicate.NewClient(replicate.WithToken(r.Token))
	if err != nil {
		fmt.Println("Error creating Replicate client")
		return ImageOutput{}, err
	}

	input := replicate.PredictionInput{
		"image": imageUrl,
	}

	webhook := replicate.Webhook{
		URL:    "https://example.com/webhook",
		Events: []replicate.WebhookEventType{"start", "completed"},
	}

	// The `Run` method is a convenience method that
	// creates a prediction, waits for it to finish, and returns the output.
	// If you want a reference to the prediction, you can call `CreatePrediction`,
	// call `Wait` on the prediction, and access its `Output` field.

	prediction, err := r8.CreatePrediction(ctx, r.ModelIdentifier, input, &webhook, false)
	if err != nil {
		// handle error
		return ImageOutput{}, err
	}

	// Wait for the prediction to finish
	err = r8.Wait(ctx, prediction)
	if err != nil {
		// handle error
		return ImageOutput{}, err
	}

	output := ImageOutput{
		ImageUrl: imageUrl,
		Caption:  splitCaption(fmt.Sprint(prediction.Output)),
	}
	return output, nil

}

func splitCaption(input string) string {
	// Split the string into two parts based on the first occurrence of ":"
	parts := strings.SplitN(input, ":", 2)

	// Check if the split resulted in at least two parts
	if len(parts) < 2 {
		fmt.Println("No caption found")
		return ""
	}

	// Trim leading and trailing whitespace from the second part
	caption := strings.TrimSpace(parts[1])

	return caption
}
