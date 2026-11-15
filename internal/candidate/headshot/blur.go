package headshot

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"

	"github.com/disintegration/imaging"
)

// blurSigma is the Gaussian standard deviation (imaging.Blur's sigma, not a pixel radius)
// applied to a stored headshot before it is served publicly. Large enough that no facial
// feature survives as a distinct shape; chosen by eye since "unrecognizable" is a visual
// judgment. Fixed: nothing justifies tuning how anonymous a public photo is.
const blurSigma = 30

// Blur returns a strongly, irreversibly blurred rendering of a stored headshot. It never
// returns the input unchanged or partially blurs. Used only on the public read path; the
// stored original is never touched.
func Blur(data []byte) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedImage, err)
	}

	blurred := imaging.Blur(src, blurSigma)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, blurred, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("headshot: encode blurred: %w", err)
	}
	return out.Bytes(), nil
}
