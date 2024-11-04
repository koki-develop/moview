package resize

import (
	"image"

	"github.com/nfnt/resize"
	"github.com/qeesung/image2ascii/convert"
)

type Resizer struct {
	resizeHandler *convert.ImageResizeHandler
}

func NewResizer() *Resizer {
	return &Resizer{
		resizeHandler: convert.NewResizeHandler().(*convert.ImageResizeHandler),
	}
}

func (r *Resizer) Resize(img image.Image, w, h int) image.Image {
	sz := img.Bounds()
	neww, newh := r.resizeHandler.CalcFitSize(float64(w), float64(h), float64(sz.Max.X), float64(sz.Max.Y))
	return resize.Resize(uint(neww), uint(newh), img, resize.Lanczos3)
}
