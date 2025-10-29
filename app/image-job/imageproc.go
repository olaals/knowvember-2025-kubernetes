package main

import (
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

func toNRGBA(img image.Image) *image.NRGBA {
	if nrgba, ok := img.(*image.NRGBA); ok {
		return nrgba
	}
	b := img.Bounds()
	dst := image.NewNRGBA(b)
	draw.Draw(dst, b, img, b.Min, draw.Src)
	return dst
}

func toGrayscale(img image.Image) image.Image {
	src := toNRGBA(img)
	b := src.Bounds()
	gray := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		i := (y - b.Min.Y) * src.Stride
		for x := b.Min.X; x < b.Max.X; x++ {
			r := src.Pix[i+0]
			g := src.Pix[i+1]
			bl := src.Pix[i+2]
			a := src.Pix[i+3]
			l := uint8((299*uint32(r) + 587*uint32(g) + 114*uint32(bl) + 500) / 1000)
			gray.Pix[i+0] = l
			gray.Pix[i+1] = l
			gray.Pix[i+2] = l
			gray.Pix[i+3] = a
			i += 4
		}
	}
	return gray
}

func invertColors(img image.Image) image.Image {
	src := toNRGBA(img)
	b := src.Bounds()
	dst := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		i := (y - b.Min.Y) * src.Stride
		for x := b.Min.X; x < b.Max.X; x++ {
			r := src.Pix[i+0]
			g := src.Pix[i+1]
			bl := src.Pix[i+2]
			a := src.Pix[i+3]
			dst.Pix[i+0] = 255 - r
			dst.Pix[i+1] = 255 - g
			dst.Pix[i+2] = 255 - bl
			dst.Pix[i+3] = a
			i += 4
		}
	}
	return dst
}
