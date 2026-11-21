package extract

import (
	"context"
	"errors"
	"fmt"

	vision "cloud.google.com/go/vision/v2/apiv1"
	"cloud.google.com/go/vision/v2/apiv1/visionpb"
	"google.golang.org/api/option"
)

var ErrVisionPerImage = errors.New("vision per-image error")

// ImageOCR extracts text from an image using GCP Vision API.
func ImageOCR(ctx context.Context, data []byte, gcpProject string) (string, error) {
	client, err := vision.NewImageAnnotatorClient(ctx,
		option.WithQuotaProject(gcpProject),
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
