package ocr

import (
	"context"
	"errors"
	"fmt"

	vision "cloud.google.com/go/vision/v2/apiv1"
	"cloud.google.com/go/vision/v2/apiv1/visionpb"
	"google.golang.org/api/option"
)

// ErrVisionPerImage is returned when the Vision API reports a per-image error.
var ErrVisionPerImage = errors.New("vision per-image error")

// GCPVision is an OCR provider backed by the Google Cloud Vision API.
type GCPVision struct {
	project string
}

// NewGCPVision creates a new GCPVision provider for the given GCP project.
func NewGCPVision(project string) *GCPVision {
	return &GCPVision{project: project}
}

// ExtractText implements Provider using GCP Vision API text detection.
func (g *GCPVision) ExtractText(ctx context.Context, data []byte) (string, error) {
	client, err := vision.NewImageAnnotatorClient(ctx,
		option.WithQuotaProject(g.project),
	)
	if err != nil {
		return "", fmt.Errorf("create vision client: %w", err)
	}
	defer client.Close()

	resp, err := client.BatchAnnotateImages(ctx, &visionpb.BatchAnnotateImagesRequest{
		Requests: []*visionpb.AnnotateImageRequest{
			{
				Image: &visionpb.Image{Content: data},
				Features: []*visionpb.Feature{
					{Type: visionpb.Feature_TEXT_DETECTION},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("annotate image: %w", err)
	}
	if len(resp.Responses) == 0 {
		return "", nil
	}
	r := resp.Responses[0]
	if r.Error != nil && r.Error.Code != 0 {
		return "", fmt.Errorf("%w: %s", ErrVisionPerImage, r.Error.Message)
	}
	if r.FullTextAnnotation == nil {
		return "", nil
	}
	return r.FullTextAnnotation.Text, nil
}
