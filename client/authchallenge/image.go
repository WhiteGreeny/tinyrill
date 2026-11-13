package authchallenge

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"strings"
)

const maxImageBytes = 2 << 20

func imageSize(data []byte) (image.Config, error) {
	if len(data) == 0 || len(data) > maxImageBytes {
		return image.Config{}, errors.New("captcha image size limit exceeded")
	}
	c, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return c, errors.New("invalid captcha image")
	}
	if c.Width < 1 || c.Height < 1 || c.Width > 4096 || c.Height > 4096 || c.Width*c.Height > 4<<20 {
		return c, errors.New("captcha image dimensions exceed limit")
	}
	return c, nil
}

func validateImagePoints(data []byte, response ClickCaptchaResponse) error {
	c, err := imageSize(data)
	if err != nil {
		return err
	}
	w, h := response.Width, response.Height
	if w == 0 && h == 0 {
		w, h = c.Width, c.Height
	}
	if w < 1 || h < 1 || w > 4096 || h > 4096 || len(response.Points) < 1 || len(response.Points) > 16 {
		return errors.New("invalid click captcha dimensions or point count")
	}
	for _, p := range response.Points {
		if p.X < 0 || p.Y < 0 || p.X >= w || p.Y >= h {
			return errors.New("click captcha point is outside the image")
		}
	}
	return nil
}

func displayImage(output io.Writer, data []byte, mode string) error {
	c, err := imageSize(data)
	if err != nil {
		return err
	}
	if mode == "auto" && os.Getenv("TERM_PROGRAM") == "iTerm.app" {
		fmt.Fprintf(output, "\x1b]1337;File=inline=1;size=%d:%s\a\n", len(data), base64.StdEncoding.EncodeToString(data))
		return nil
	}
	// ANSI half blocks preserve colour. The original protected file is also
	// available when fine lines are lost at the terminal's resolution.
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return errors.New("decode captcha image failed")
	}
	w := min(c.Width, 100)
	h := max(2, c.Height*w/c.Width)
	if h > 100 {
		w = max(1, w*100/h)
		h = 100
	}
	var line strings.Builder
	for y := 0; y < h; y += 2 {
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(x*c.Width/w, y*c.Height/h).RGBA()
			r2, g2, b2, _ := img.At(x*c.Width/w, min(y+1, h-1)*c.Height/h).RGBA()
			fmt.Fprintf(&line, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", r>>8, g>>8, b>>8, r2>>8, g2>>8, b2>>8)
		}
		line.WriteString("\x1b[0m\n")
	}
	_, err = io.WriteString(output, line.String())
	return err
}
