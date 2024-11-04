package ascii

import (
	"image"
	"image/color"
	"strings"

	"github.com/qeesung/image2ascii/ascii"
)

type Converter struct {
	pixelConverter ascii.PixelConverter
}

func NewConverter() *Converter {
	return &Converter{
		pixelConverter: ascii.NewPixelConverter(),
	}
}

func (c *Converter) ImageToASCII(img image.Image) ([]string, error) {
	sz := img.Bounds()
	w := sz.Max.X
	h := sz.Max.Y

	rows := make([]string, 0, h)
	for i := 0; i < h; i++ {
		b := new(strings.Builder)
		for j := 0; j < w; j++ {
			pixel := color.NRGBAModel.Convert(img.At(j, i))
			char := c.pixelConverter.ConvertPixelToASCII(pixel, &ascii.DefaultOptions)
			b.WriteString(char)
		}
		rows = append(rows, b.String())
	}
	return rows, nil
}
